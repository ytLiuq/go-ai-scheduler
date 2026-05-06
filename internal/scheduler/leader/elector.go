package leader

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/example/go-ai-scheduler/internal/pkg/metrics"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// Elector grants one scheduler process permission to run leader-only loops.
type Elector interface {
	Acquire(context.Context) error
}

// New creates a leader elector for the current backend.
func New(db *sql.DB, etcdAddrs []string, logger *slog.Logger) Elector {
	if len(etcdAddrs) > 0 {
		return newEtcdElector(etcdAddrs, logger)
	}
	if db == nil {
		return &localElector{logger: logger}
	}
	return &mysqlElector{
		db:       db,
		logger:   logger,
		lockName: "go-ai-scheduler/leader",
	}
}

type localElector struct {
	logger *slog.Logger
}

func (e *localElector) Acquire(_ context.Context) error {
	e.logger.Debug("leader election", "backend", "local", "role", "leader")
	metrics.DefaultRegistry.IncCounter("leader_election_total", map[string]string{
		"backend": "local",
		"result":  "acquired",
	})
	return nil
}

type mysqlElector struct {
	db       *sql.DB
	logger   *slog.Logger
	lockName string
}

func (e *mysqlElector) Acquire(ctx context.Context) error {
	for {
		conn, err := e.db.Conn(ctx)
		if err != nil {
			return fmt.Errorf("open leader election connection: %w", err)
		}

		var acquired int
		err = conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", e.lockName).Scan(&acquired)
		if err == nil && acquired == 1 {
			e.logger.Debug("leader election", "backend", "mysql", "role", "leader")
			metrics.DefaultRegistry.IncCounter("leader_election_total", map[string]string{
				"backend": "mysql",
				"result":  "acquired",
			})
			go func() {
				<-ctx.Done()
				_, _ = conn.ExecContext(context.Background(), "DO RELEASE_LOCK(?)", e.lockName)
				_ = conn.Close()
			}()
			return nil
		}

		_ = conn.Close()
		if err != nil {
			return fmt.Errorf("acquire mysql leader lock: %w", err)
		}

		metrics.DefaultRegistry.IncCounter("leader_election_total", map[string]string{
			"backend": "mysql",
			"result":  "contended",
		})
		e.logger.Debug("leader election", "backend", "mysql", "role", "follower", "state", "waiting_for_lock")

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

type etcdElector struct {
	client *clientv3.Client
	logger *slog.Logger
	prefix string
}

func newEtcdElector(addrs []string, logger *slog.Logger) Elector {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   addrs,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		logger.Warn("failed to create etcd client, falling back to local", "error", err)
		return &localElector{logger: logger}
	}
	return &etcdElector{
		client: cli,
		logger: logger,
		prefix: "/go-ai-scheduler/leader",
	}
}

func (e *etcdElector) Acquire(ctx context.Context) error {
	session, err := concurrency.NewSession(e.client, concurrency.WithTTL(5))
	if err != nil {
		e.logger.Warn("etcd session creation failed, falling back to local", "error", err)
		e.client.Close()
		return (&localElector{logger: e.logger}).Acquire(ctx)
	}

	election := concurrency.NewElection(session, e.prefix)
	if err := election.Campaign(ctx, "scheduler"); err != nil {
		session.Close()
		e.logger.Warn("etcd campaign failed, falling back to local", "error", err)
		e.client.Close()
		return (&localElector{logger: e.logger}).Acquire(ctx)
	}

	e.logger.Debug("leader election", "backend", "etcd", "role", "leader")
	metrics.DefaultRegistry.IncCounter("leader_election_total", map[string]string{
		"backend": "etcd",
		"result":  "acquired",
	})

	go func() {
		<-ctx.Done()
		_ = election.Resign(context.Background())
		session.Close()
		e.client.Close()
	}()

	return nil
}
