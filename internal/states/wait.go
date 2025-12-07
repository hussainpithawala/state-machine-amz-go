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
		return fmt.Errorf("wait state '%s' must specify one of: Seconds, SecondsPath, Timestamp, or TimestampPath", w.Name)
	}

	if waitMethodsCount > 1 {
		return fmt.Errorf("wait state '%s' must specify only one of: Seconds, SecondsPath, Timestamp, or TimestampPath", w.Name)
	}

	// Validate Seconds value if present
	if w.Seconds != nil && *w.Seconds < 0 {
		return fmt.Errorf("wait state '%s' Seconds must be non-negative", w.Name)
	}

	return nil
}

// Execute executes the Wait state
func (w *WaitState) Execute(ctx context.Context, input interface{}) (resultOutput interface{}, nextState *string, err error) {
	// Apply input path
	processor, processedInput, err := w.prepareInput(input)
	if err != nil {
		return nil, nil, err
	}

	// Determine wait duration
	waitDuration, err := w.calculateWaitDuration(processor, processedInput)
	if err != nil {
		return nil, nil, err
	}

	// Perform the wait
	if err := w.performWait(ctx, waitDuration); err != nil {
		return nil, nil, err
	}

	// Apply output paths and return result
	return w.prepareOutput(processor, processedInput)
}

// prepareInput applies input path to the input
func (w *WaitState) prepareInput(input interface{}) (*JSONPathProcessor, interface{}, error) {
	processor := NewJSONPathProcessor()
	processedInput, err := processor.ApplyInputPath(input, w.InputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply input path: %w", err)
	}
	return processor, processedInput, nil
}

// calculateWaitDuration determines how long to wait based on state configuration
func (w *WaitState) calculateWaitDuration(processor *JSONPathProcessor, processedInput interface{}) (time.Duration, error) {
	// Define wait strategies
	strategies := []struct {
		condition bool
		waitFunc  func() (time.Duration, error)
	}{
		{
			w.Seconds != nil,
			func() (time.Duration, error) {
				return time.Duration(*w.Seconds) * time.Second, nil
			},
		},
		{
			w.SecondsPath != nil,
			func() (time.Duration, error) {
				return w.calculateSecondsPathWait(processor, processedInput)
			},
		},
		{
			w.Timestamp != nil,
			func() (time.Duration, error) {
				return w.calculateTimestampWait(*w.Timestamp)
			},
		},
		{
			w.TimestampPath != nil,
			func() (time.Duration, error) {
				return w.calculateTimestampPathWait(processor, processedInput)
			},
		},
	}

	// Find and execute the first matching strategy
	for _, strategy := range strategies {
		if strategy.condition {
			return strategy.waitFunc()
		}
	}

	// Default: no wait
	return 0, nil
}

// calculateSecondsPathWait calculates wait duration from SecondsPath
func (w *WaitState) calculateSecondsPathWait(processor *JSONPathProcessor, processedInput interface{}) (time.Duration, error) {
	secondsValue, err := processor.Get(processedInput, *w.SecondsPath)
	if err != nil {
		return 0, fmt.Errorf("failed to extract SecondsPath '%s': %w", *w.SecondsPath, err)
	}

	seconds, err := toInt64(secondsValue)
	if err != nil {
		return 0, fmt.Errorf("SecondsPath value is not a valid number: %w", err)
	}

	if seconds < 0 {
		return 0, fmt.Errorf("SecondsPath value must be non-negative")
	}

	return time.Duration(seconds) * time.Second, nil
}

// calculateTimestampWait calculates wait duration until a timestamp
func (w *WaitState) calculateTimestampWait(timestamp string) (time.Duration, error) {
	targetTime, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return 0, fmt.Errorf("failed to parse timestamp '%s': %w", timestamp, err)
	}

	return w.calculateTimeUntil(targetTime), nil
}

// calculateTimestampPathWait calculates wait duration from TimestampPath
func (w *WaitState) calculateTimestampPathWait(processor *JSONPathProcessor, processedInput interface{}) (time.Duration, error) {
	timestampValue, err := processor.Get(processedInput, *w.TimestampPath)
	if err != nil {
		return 0, fmt.Errorf("failed to extract TimestampPath '%s': %w", *w.TimestampPath, err)
	}

	timestampStr, ok := timestampValue.(string)
	if !ok {
		return 0, fmt.Errorf("TimestampPath value is not a string")
	}

	return w.calculateTimestampWait(timestampStr)
}

// calculateTimeUntil calculates duration until a target time
func (w *WaitState) calculateTimeUntil(targetTime time.Time) time.Duration {
	waitDuration := time.Until(targetTime)
	if waitDuration < 0 {
		// Timestamp is in the past, don't wait
		return 0
	}
	return waitDuration
}

// performWait waits for the specified duration or until context is cancelled
func (w *WaitState) performWait(ctx context.Context, waitDuration time.Duration) error {
	if waitDuration <= 0 {
		return nil // No wait needed
	}

	timer := time.NewTimer(waitDuration)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil // Wait completed
	case <-ctx.Done():
		return ctx.Err() // Context cancelled
	}
}

// prepareOutput applies result and output paths to create the final output
func (w *WaitState) prepareOutput(processor *JSONPathProcessor, processedInput interface{}) (resultOutput interface{}, nextState *string, err error) {
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
