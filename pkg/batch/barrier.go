package batch

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// ──────────────────────────────────────────────────────────────────────────────
// Barrier – Distributed Fork-Join via Lua
//
// Init: sets a counter to N (micro-batch size) using SET NX (idempotent).
// Decrement: atomically DECR and returns remaining.  The caller who receives 0
// is the "winner" responsible for resuming the orchestrator state machine.
//
// Both operations are Lua scripts executed atomically inside Redis so that
// exactly one worker is declared the winner even when all N workers complete
// within microseconds of each other.
// ──────────────────────────────────────────────────────────────────────────────

// barrierDecrScript atomically decrements and returns the new counter value.
//
//	KEYS[1] – barrier key
//	ARGV[1] – TTL refresh value (seconds)
//
// Returns:
//
//	≥1   other workers still running
//	 0   this caller is the last worker (winner)
//	-1   key was missing (guard against retry / expiry race)
const barrierDecrScript = `
local key = KEYS[1]
local ttl = tonumber(ARGV[1])
if redis.call("EXISTS", key) == 0 then return -1 end
local rem = redis.call("DECR", key)
if rem > 0 then redis.call("EXPIRE", key, ttl) end
if rem == 0 then redis.call("DEL", key) end
return rem
`

// barrierInitScript initialises the counter using SET NX (set-if-not-exists).
//
//	KEYS[1] – barrier key
//	ARGV[1] – initial count
//	ARGV[2] – TTL (seconds)
//
// Returns: 1 if initialised, 0 if already existed (idempotent).
const barrierInitScript = `
local key = KEYS[1]
local ok = redis.call("SET", key, ARGV[1], "NX", "EX", tonumber(ARGV[2]))
return ok and 1 or 0
`

// Barrier manages the Redis fork-join barrier for micro-batch synchronisation.
type Barrier struct {
	rdb     *redis.Client
	decrSHA string
	initSHA string
}

// NewBarrier creates a Barrier and eagerly loads both Lua scripts into Redis.
func NewBarrier(ctx context.Context, rdb *redis.Client) (*Barrier, error) {
	b := &Barrier{rdb: rdb}
	var err error

	b.decrSHA, err = rdb.ScriptLoad(ctx, barrierDecrScript).Result()
	if err != nil {
		return nil, fmt.Errorf("batch/barrier: load decr script: %w", err)
	}

	b.initSHA, err = rdb.ScriptLoad(ctx, barrierInitScript).Result()
	if err != nil {
		return nil, fmt.Errorf("batch/barrier: load init script: %w", err)
	}

	return b, nil
}

// Init initialises the barrier counter for a micro-batch.  Idempotent.
func (b *Barrier) Init(ctx context.Context, mbID string, size int) error {
	_, err := b.rdb.EvalSha(ctx, b.initSHA,
		[]string{keyBarrier(mbID)},
		size, int(BarrierTTL.Seconds()),
	).Result()
	if err != nil {
		return fmt.Errorf("batch/barrier: init %q: %w", mbID, err)
	}
	return nil
}

// Decrement atomically decrements the barrier and reports whether this caller
// is the last worker (counter reached 0).
func (b *Barrier) Decrement(ctx context.Context, mbID string) (BarrierResult, error) {
	rem, err := b.rdb.EvalSha(ctx, b.decrSHA,
		[]string{keyBarrier(mbID)},
		int(BarrierTTL.Seconds()),
	).Int64()
	if err != nil {
		return BarrierResult{}, fmt.Errorf("batch/barrier: decrement %q: %w", mbID, err)
	}
	return BarrierResult{Remaining: rem, IsLastWorker: rem == 0}, nil
}

// Remaining returns the current counter value for observability.
func (b *Barrier) Remaining(ctx context.Context, mbID string) (int64, error) {
	v, err := b.rdb.Get(ctx, keyBarrier(mbID)).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("batch/barrier: remaining %q: %w", mbID, err)
	}
	return v, nil
}
