package flow_locker

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/redigo"
	"github.com/gofrs/uuid"
	"github.com/gomodule/redigo/redis"
)

// RedisLocker implements FlowLocker using Redis with Redlock
type RedisLocker struct {
	rs     *redsync.Redsync
	expiry time.Duration
}

// RedisLockerConfig holds configuration for RedisLocker
type RedisLockerConfig struct {
	Address  string
	Password string
	Expiry   time.Duration
}

// NewRedisLocker creates a new Redis-based flow locker
func NewRedisLocker(config RedisLockerConfig) *RedisLocker {
	if config.Expiry == 0 {
		config.Expiry = 15 * time.Second
	}

	pool := &redis.Pool{
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", config.Address, redis.DialPassword(config.Password))
			if err != nil {
				return nil, err
			}

			return c, nil
		},
	}

	rs := redsync.New(redigo.NewPool(pool))

	return &RedisLocker{
		rs:     rs,
		expiry: config.Expiry,
	}
}

// Lock acquires a distributed lock for the given flow ID
func (r *RedisLocker) Lock(ctx context.Context, flowID uuid.UUID) (func(context.Context) error, error) {
	// Check if context is already canceled before attempting lock
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context error: %w", err)
	}

	mutex := r.rs.NewMutex(
		"flow:lock:"+flowID.String(),
		redsync.WithExpiry(r.expiry),
		redsync.WithTries(1),
	)

	if err := mutex.LockContext(ctx); err != nil {
		return nil, err
	}

	unlock := func(ctx context.Context) error {
		if ok, err := mutex.UnlockContext(ctx); !ok || err != nil {
			return fmt.Errorf("failed to release lock: %w", err)
		}

		return nil
	}

	return unlock, nil
}
