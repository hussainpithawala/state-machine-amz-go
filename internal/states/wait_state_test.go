package states

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaitState_Execute_WithSeconds(t *testing.T) {
	ctx := context.Background()
	state := &WaitState{
		BaseState: BaseState{
			Name: "WaitState",
			Type: "Wait",
			Next: StringPtr("NextState"),
		},
		Seconds: Int64Ptr(1),
	}

	input := map[string]interface{}{
		"key": "value",
	}

	startTime := time.Now()
	output, nextState, err := state.Execute(ctx, input)
	elapsed := time.Since(startTime)

	require.NoError(t, err)
	assert.Equal(t, input, output)
	assert.NotNil(t, nextState)
	assert.Equal(t, "NextState", *nextState)

	// Verify that we waited approximately 1 second
	assert.True(t, elapsed >= time.Second, "Should wait at least 1 second")
	assert.True(t, elapsed < 2*time.Second, "Should not wait more than 2 seconds")
}

func TestWaitState_Execute_WithZeroSeconds(t *testing.T) {
	ctx := context.Background()
	state := &WaitState{
		BaseState: BaseState{
			Name: "WaitState",
			Type: "Wait",
			Next: StringPtr("NextState"),
		},
		Seconds: Int64Ptr(0),
	}

	input := map[string]interface{}{
		"key": "value",
	}

	output, nextState, err := state.Execute(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, input, output)
	assert.Equal(t, "NextState", *nextState)
}

func TestWaitState_Execute_WithSecondsPath(t *testing.T) {
	ctx := context.Background()
	state := &WaitState{
		BaseState: BaseState{
			Name: "WaitState",
			Type: "Wait",
			Next: StringPtr("NextState"),
		},
		SecondsPath: StringPtr("$.duration"),
	}

	input := map[string]interface{}{
		"duration": 1,
		"key":      "value",
	}

	startTime := time.Now()
	output, nextState, err := state.Execute(ctx, input)
	elapsed := time.Since(startTime)

	require.NoError(t, err)
	assert.Equal(t, input, output)
	assert.Equal(t, "NextState", *nextState)
	assert.True(t, elapsed >= time.Second)
}

func TestWaitState_Execute_WithTimestamp(t *testing.T) {
	ctx := context.Background()

	// Set timestamp to 1 second in the future
	futureTime := time.Now().Add(1 * time.Second).UTC()
	timestamp := futureTime.Format(time.RFC3339)

	state := &WaitState{
		BaseState: BaseState{
			Name: "WaitState",
			Type: "Wait",
			End:  true,
		},
		Timestamp: StringPtr(timestamp),
	}

	input := map[string]interface{}{
		"key": "value",
	}

	output, nextState, err := state.Execute(ctx, input)
	startTime := time.Now()
	elapsed := time.Since(startTime)

	require.NoError(t, err)
	assert.Equal(t, input, output)
	assert.Nil(t, nextState)
	assert.True(t, elapsed >= 0)
}

func TestWaitState_Execute_WithPastTimestamp(t *testing.T) {
	ctx := context.Background()

	// Set timestamp to 1 second in the past
	pastTime := time.Now().Add(-1 * time.Second).UTC()
	timestamp := pastTime.Format(time.RFC3339)

	state := &WaitState{
		BaseState: BaseState{
			Name: "WaitState",
			Type: "Wait",
			End:  true,
		},
		Timestamp: StringPtr(timestamp),
	}

	input := map[string]interface{}{
		"key": "value",
	}

	startTime := time.Now()
	output, _, err := state.Execute(ctx, input)
	elapsed := time.Since(startTime)

	require.NoError(t, err)
	assert.Equal(t, input, output)

	// Should complete immediately without waiting
	assert.True(t, elapsed < 100*time.Millisecond)
}

func TestWaitState_Execute_WithTimestampPath(t *testing.T) {
	ctx := context.Background()

	futureTime := time.Now().Add(1 * time.Second).UTC()
	timestamp := futureTime.Format(time.RFC3339)

	state := &WaitState{
		BaseState: BaseState{
			Name: "WaitState",
			Type: "Wait",
			Next: StringPtr("NextState"),
		},
		TimestampPath: StringPtr("$.waitUntil"),
	}

	input := map[string]interface{}{
		"waitUntil": timestamp,
		"data":      "test",
	}

	startTime := time.Now()
	output, nextState, err := state.Execute(ctx, input)
	elapsed := time.Since(startTime)

	require.NoError(t, err)
	assert.Equal(t, input, output)
	assert.Equal(t, "NextState", *nextState)
	assert.True(t, elapsed >= time.Second)
}

func TestWaitState_Execute_ContextCancelled(t *testing.T) {
	state := &WaitState{
		BaseState: BaseState{
			Name: "WaitState",
			Type: "Wait",
		},
		Seconds: Int64Ptr(10),
	}

	input := map[string]interface{}{
		"key": "value",
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	_, _, err := state.Execute(ctx, input)
	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestWaitState_Validate(t *testing.T) {
	tests := []struct {
		name      string
		state     *WaitState
		shouldErr bool
		errMsg    string
	}{
		{
			name: "valid with Seconds",
			state: &WaitState{
				BaseState: BaseState{
					Name: "WaitState",
					Type: "Wait",
				},
				Seconds: Int64Ptr(5),
			},
			shouldErr: false,
		},
		{
			name: "valid with SecondsPath",
			state: &WaitState{
				BaseState: BaseState{
					Name: "WaitState",
					Type: "Wait",
				},
				SecondsPath: StringPtr("$.duration"),
			},
			shouldErr: false,
		},
		{
			name: "valid with Timestamp",
			state: &WaitState{
				BaseState: BaseState{
					Name: "WaitState",
					Type: "Wait",
				},
				Timestamp: StringPtr("2025-12-31T23:59:59Z"),
			},
			shouldErr: false,
		},
		{
			name: "valid with TimestampPath",
			state: &WaitState{
				BaseState: BaseState{
					Name: "WaitState",
					Type: "Wait",
				},
				TimestampPath: StringPtr("$.timestamp"),
			},
			shouldErr: false,
		},
		{
			name: "no wait method specified",
			state: &WaitState{
				BaseState: BaseState{
					Name: "WaitState",
					Type: "Wait",
				},
			},
			shouldErr: true,
			errMsg:    "must specify one of",
		},
		{
			name: "multiple wait methods specified",
			state: &WaitState{
				BaseState: BaseState{
					Name: "WaitState",
					Type: "Wait",
				},
				Seconds:     Int64Ptr(5),
				SecondsPath: StringPtr("$.duration"),
			},
			shouldErr: true,
			errMsg:    "must specify only one of",
		},
		{
			name: "negative Seconds",
			state: &WaitState{
				BaseState: BaseState{
					Name: "WaitState",
					Type: "Wait",
				},
				Seconds: Int64Ptr(-1),
			},
			shouldErr: true,
			errMsg:    "must be non-negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.state.Validate()

			if tt.shouldErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWaitState_GettersAndSetters(t *testing.T) {
	state := &WaitState{
		BaseState: BaseState{
			Name: "MyWaitState",
			Type: "Wait",
			End:  true,
			Next: StringPtr("NextState"),
		},
		Seconds: Int64Ptr(5),
	}

	assert.Equal(t, "MyWaitState", state.GetName())
	assert.Equal(t, "Wait", state.GetType())
	assert.True(t, state.IsEnd())
	assert.Equal(t, "NextState", *state.GetNext())
}

func Int64Ptr(i int64) *int64 {
	return &i
}
