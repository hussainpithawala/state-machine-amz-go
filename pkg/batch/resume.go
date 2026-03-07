package batch

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ResumeController handles the human-in-the-loop pause/resume cycle.
//
// The PauseBatch → CheckResumeSignal loop polls a Redis key that an operator
// sets to unblock processing.  The CheckResumeSignal task handler calls
// ResumeController.Check, which uses GETDEL to atomically consume the signal
// so the loop cannot re-enter on the same signal.
type ResumeController struct {
	rdb *redis.Client
}

// NewResumeController returns a ResumeController backed by the given Redis client.
func NewResumeController(rdb *redis.Client) *ResumeController {
	return &ResumeController{rdb: rdb}
}

// Signal sets the resume flag for a paused batch.  Call this from an internal
// ops handler, CLI tool, or automated remediation system.
func (r *ResumeController) Signal(ctx context.Context, batchID, operator, notes string) error {
	sig := ResumeSignal{
		ResumedAt: time.Now().UTC(),
		ResumedBy: operator,
		Notes:     notes,
	}
	data, err := json.Marshal(sig)
	if err != nil {
		return fmt.Errorf("resume: marshal: %w", err)
	}
	return r.rdb.Set(ctx, keyResume(batchID), string(data), ResumeTTL).Err()
}

// Revoke removes an unconsumed resume signal (operator changed their mind).
func (r *ResumeController) Revoke(ctx context.Context, batchID string) error {
	return r.rdb.Del(ctx, keyResume(batchID)).Err()
}

// Check atomically reads-and-deletes the resume signal.
//
// Returns ResumeCheckResult{ShouldResume: true} if a valid signal was present,
// or {ShouldResume: false} if none exists (keep polling).
func (r *ResumeController) Check(ctx context.Context, batchID string) (ResumeCheckResult, error) {
	data, err := r.rdb.GetDel(ctx, keyResume(batchID)).Result()
	if err == redis.Nil {
		return ResumeCheckResult{ShouldResume: false}, nil
	}
	if err != nil {
		return ResumeCheckResult{}, fmt.Errorf("resume: getdel: %w", err)
	}
	var sig ResumeSignal
	if err := json.Unmarshal([]byte(data), &sig); err != nil {
		// Malformed – treat as no signal to avoid an infinite pause.
		return ResumeCheckResult{ShouldResume: false}, nil
	}
	return ResumeCheckResult{
		ShouldResume: true,
		ResumedAt:    &sig.ResumedAt,
		ResumedBy:    sig.ResumedBy,
	}, nil
}
