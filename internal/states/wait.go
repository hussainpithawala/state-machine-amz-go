package states

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// WaitState represents an AWS Wait state
// https://docs.aws.amazon.com/step-functions/latest/dg/concepts-tasks-await-response.html
type WaitState struct {
	BaseState
	Seconds       *int64  `json:"Seconds,omitempty"`
	SecondsPath   *string `json:"SecondsPath,omitempty"`
	Timestamp     *string `json:"Timestamp,omitempty"`
	TimestampPath *string `json:"TimestampPath,omitempty"`
}

// GetName returns the name of the state
func (w *WaitState) GetName() string {
	return w.Name
}

// GetType returns the type of the state
func (w *WaitState) GetType() string {
	return "Wait"
}

// GetNext returns the next state name
func (w *WaitState) GetNext() *string {
	return w.Next
}

// IsEnd returns true if this is an end state
func (w *WaitState) IsEnd() bool {
	return w.End
}

// Validate validates the Wait state configuration
func (w *WaitState) Validate() error {
	// At least one wait method must be specified
	waitMethodsCount := 0
	if w.Seconds != nil {
		waitMethodsCount++
	}
	if w.SecondsPath != nil {
		waitMethodsCount++
	}
	if w.Timestamp != nil {
		waitMethodsCount++
	}
	if w.TimestampPath != nil {
		waitMethodsCount++
	}

	if waitMethodsCount == 0 {
		return fmt.Errorf("Wait state '%s' must specify one of: Seconds, SecondsPath, Timestamp, or TimestampPath", w.Name)
	}

	if waitMethodsCount > 1 {
		return fmt.Errorf("Wait state '%s' must specify only one of: Seconds, SecondsPath, Timestamp, or TimestampPath", w.Name)
	}

	// Validate Seconds value if present
	if w.Seconds != nil && *w.Seconds < 0 {
		return fmt.Errorf("Wait state '%s' Seconds must be non-negative", w.Name)
	}

	return nil
}

// Execute executes the Wait state
func (w *WaitState) Execute(ctx context.Context, input interface{}) (interface{}, *string, error) {
	// Apply input path
	processor := NewJSONPathProcessor()
	processedInput, err := processor.ApplyInputPath(input, w.InputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply input path: %w", err)
	}

	// Determine the wait duration
	var waitDuration time.Duration

	switch {
	case w.Seconds != nil:
		// Fixed duration in seconds
		waitDuration = time.Duration(*w.Seconds) * time.Second

	case w.SecondsPath != nil:
		// Extract seconds from input
		secondsValue, err := processor.Get(processedInput, *w.SecondsPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to extract SecondsPath '%s': %w", *w.SecondsPath, err)
		}

		// Convert to int64
		seconds, err := toInt64(secondsValue)
		if err != nil {
			return nil, nil, fmt.Errorf("SecondsPath value is not a valid number: %w", err)
		}

		if seconds < 0 {
			return nil, nil, fmt.Errorf("SecondsPath value must be non-negative")
		}

		waitDuration = time.Duration(seconds) * time.Second

	case w.Timestamp != nil:
		// Wait until a specific timestamp
		targetTime, err := time.Parse(time.RFC3339, *w.Timestamp)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse Timestamp '%s': %w", *w.Timestamp, err)
		}

		waitDuration = time.Until(targetTime)
		if waitDuration < 0 {
			// Timestamp is in the past, don't wait
			waitDuration = 0
		}

	case w.TimestampPath != nil:
		// Extract timestamp from input
		timestampValue, err := processor.Get(processedInput, *w.TimestampPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to extract TimestampPath '%s': %w", *w.TimestampPath, err)
		}

		// Convert to string and parse
		timestampStr, ok := timestampValue.(string)
		if !ok {
			return nil, nil, fmt.Errorf("TimestampPath value is not a string")
		}

		targetTime, err := time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse TimestampPath value '%s': %w", timestampStr, err)
		}

		waitDuration = time.Until(targetTime)
		if waitDuration < 0 {
			// Timestamp is in the past, don't wait
			waitDuration = 0
		}
	}

	// Wait using the context
	select {
	case <-time.After(waitDuration):
		// Wait completed normally
	case <-ctx.Done():
		// Context cancelled
		return nil, nil, ctx.Err()
	}

	// Apply result path (Wait state passes through input)
	output, err := processor.ApplyResultPath(processedInput, processedInput, w.ResultPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply result path: %w", err)
	}

	// Apply output path
	output, err = processor.ApplyOutputPath(output, w.OutputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply output path: %w", err)
	}

	return output, w.GetNext(), nil
}

// MarshalJSON implements custom JSON marshaling
func (w *WaitState) MarshalJSON() ([]byte, error) {
	type Alias WaitState
	return json.Marshal(&struct {
		Type string `json:"Type"`
		*Alias
	}{
		Type:  "Wait",
		Alias: (*Alias)(w),
	})
}

// toInt64 converts a value to int64
func toInt64(value interface{}) (int64, error) {
	switch v := value.(type) {
	case float64:
		return int64(v), nil
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case json.Number:
		return v.Int64()
	default:
		return 0, fmt.Errorf("cannot convert %T to int64", value)
	}
}
