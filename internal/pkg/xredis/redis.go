package xredis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps a Redis client with convenience methods for the scheduler.
type Client struct {
	rdb *redis.Client
}

// Open creates a new Redis connection and verifies it with PING.
func Open(ctx context.Context, addr string) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:        addr,
		DialTimeout: 5 * time.Second,
		ReadTimeout: 3 * time.Second,
		PoolSize:    20,
		MinIdleConns: 5,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &Client{rdb: rdb}, nil
}

// Close releases the connection pool.
func (c *Client) Close() error {
	return c.rdb.Close()
}

// Raw returns the underlying go-redis client for advanced operations.
func (c *Client) Raw() *redis.Client {
	return c.rdb
}

// ---- task due-time cache ----

const taskDueSetKey = "scheduler:due:tasks"
const taskDueZSetKey = "scheduler:due:zset"

// WarmDueTasks replaces the cached set of task IDs due within the next window.
func (c *Client) WarmDueTasks(ctx context.Context, taskIDs []int64, scores map[int64]float64) error {
	pipe := c.rdb.Pipeline()
	pipe.Del(ctx, taskDueSetKey)
	pipe.Del(ctx, taskDueZSetKey)
	if len(taskIDs) > 0 {
		members := make([]any, 0, len(taskIDs))
		for _, id := range taskIDs {
			members = append(members, id)
		}
		pipe.SAdd(ctx, taskDueSetKey, members...)
		if len(scores) > 0 {
			zMembers := make([]redis.Z, 0, len(scores))
			for id, score := range scores {
				zMembers = append(zMembers, redis.Z{Score: score, Member: id})
			}
			pipe.ZAdd(ctx, taskDueZSetKey, zMembers...)
		}
	}
	pipe.Expire(ctx, taskDueSetKey, 5*time.Minute)
	pipe.Expire(ctx, taskDueZSetKey, 5*time.Minute)
	_, err := pipe.Exec(ctx)
	return err
}

// GetDueTaskIDs returns task IDs from the warm cache that are due by the given cutoff.
func (c *Client) GetDueTaskIDs(ctx context.Context, cutoff float64) ([]int64, error) {
	ids, err := c.rdb.ZRangeByScore(ctx, taskDueZSetKey, &redis.ZRangeBy{
		Min:   "0",
		Max:   fmt.Sprintf("%.0f", cutoff),
	}).Result()
	if err != nil {
		return nil, err
	}
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		var v int64
		if _, scanErr := fmt.Sscanf(id, "%d", &v); scanErr == nil {
			result = append(result, v)
		}
	}
	return result, nil
}

// ---- worker state cache ----

const workerOnlineSetKey = "scheduler:workers:online"
const workerInfoPrefix = "scheduler:worker:"

// WarmWorkerCache sets the set of online worker IDs and per-worker status hashes.
func (c *Client) WarmWorkerCache(ctx context.Context, workerIDs []string, workerData map[string]map[string]any) error {
	pipe := c.rdb.Pipeline()
	pipe.Del(ctx, workerOnlineSetKey)
	for _, id := range workerIDs {
		pipe.SAdd(ctx, workerOnlineSetKey, id)
	}
	if len(workerIDs) > 0 {
		pipe.Expire(ctx, workerOnlineSetKey, 2*time.Minute)
	}
	for id, data := range workerData {
		key := workerInfoPrefix + id
		pipe.HSet(ctx, key, data)
		pipe.Expire(ctx, key, 2*time.Minute)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// GetCachedOnlineWorkers returns the cached set of online worker IDs.
func (c *Client) GetCachedOnlineWorkers(ctx context.Context) ([]string, error) {
	return c.rdb.SMembers(ctx, workerOnlineSetKey).Result()
}

// GetCachedWorkerInfo returns cached fields for a single worker.
func (c *Client) GetCachedWorkerInfo(ctx context.Context, workerID string) (map[string]string, error) {
	return c.rdb.HGetAll(ctx, workerInfoPrefix+workerID).Result()
}

// GetCachedWorkerField returns a single cached field for a worker.
func (c *Client) GetCachedWorkerField(ctx context.Context, workerID, field string) (string, error) {
	return c.rdb.HGet(ctx, workerInfoPrefix+workerID, field).Result()
}

// ---- rate limit / concurrency slot helpers ----

const workerLoadKey = "scheduler:worker:load"

// IncrWorkerLoad atomically increments a worker's load counter and returns the new value.
func (c *Client) IncrWorkerLoad(ctx context.Context, workerID string) (int64, error) {
	val, err := c.rdb.HIncrBy(ctx, workerLoadKey, workerID, 1).Result()
	if err != nil {
		return 0, err
	}
	c.rdb.Expire(ctx, workerLoadKey, 2*time.Minute)
	return val, nil
}

// DecrWorkerLoad atomically decrements a worker's load counter.
func (c *Client) DecrWorkerLoad(ctx context.Context, workerID string) (int64, error) {
	val, err := c.rdb.HIncrBy(ctx, workerLoadKey, workerID, -1).Result()
	if err != nil {
		return 0, err
	}
	if val < 0 {
		c.rdb.HSet(ctx, workerLoadKey, workerID, 0)
		val = 0
	}
	return val, nil
}

// GetWorkerLoads returns load counts for all tracked workers.
func (c *Client) GetWorkerLoads(ctx context.Context) (map[string]int64, error) {
	raw, err := c.rdb.HGetAll(ctx, workerLoadKey).Result()
	if err != nil {
		return nil, err
	}
	result := make(map[string]int64, len(raw))
	for k, v := range raw {
		var n int64
		if _, scanErr := fmt.Sscanf(v, "%d", &n); scanErr == nil {
			result[k] = n
		}
	}
	return result, nil
}

// ---- scheduler singleton lock ----

// TryAcquireSchedulerLock attempts to set a Redis key as a scheduler singleton lock.
func (c *Client) TryAcquireSchedulerLock(ctx context.Context, schedulerID string, ttl time.Duration) (bool, error) {
	return c.rdb.SetNX(ctx, "scheduler:singleton:lock", schedulerID, ttl).Result()
}

// RenewSchedulerLock extends the TTL of the singleton lock if still held by this schedulerID.
func (c *Client) RenewSchedulerLock(ctx context.Context, schedulerID string, ttl time.Duration) error {
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("EXPIRE", KEYS[1], ARGV[2])
		end
		return 0
	`
	return c.rdb.Eval(ctx, script, []string{"scheduler:singleton:lock"}, schedulerID, int(ttl.Seconds())).Err()
}
