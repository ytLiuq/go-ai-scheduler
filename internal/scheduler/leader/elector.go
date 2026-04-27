package leader

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/example/go-ai-scheduler/internal/pkg/metrics"
)

// Elector grants one scheduler process permission to run leader-only loops.
type Elector interface {
	Acquire(context.Context) error
}

// New creates a leader elector for the current repository backend.
func New(db *sql.DB, logger *log.Logger) Elector {
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
	logger *log.Logger
}

func (e *localElector) Acquire(_ context.Context) error {
	e.logger.Printf("leader election backend=local role=leader")
	metrics.DefaultRegistry.IncCounter("leader_election_total", map[string]string{
		"backend": "local",
		"result":  "acquired",
	})
	return nil
}

type mysqlElector struct {
	db       *sql.DB
	logger   *log.Logger
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
			e.logger.Printf("leader election backend=mysql role=leader")
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
		e.logger.Printf("leader election backend=mysql role=follower waiting_for_lock")

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
