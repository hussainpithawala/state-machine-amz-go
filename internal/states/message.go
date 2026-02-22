package states

import (
	"context"
	"fmt"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/types"
)

// ReceivedMessageBase is the prefix for received message keys in state data.
const ReceivedMessageBase = "__received_message__"

// MessageState represents a state that pauses execution and waits for an external message
type MessageState struct {
	BaseState
	// CorrelationKey is the key used to correlate incoming messages with this waiting state
	CorrelationKey string `json:"CorrelationKey"`

	// CorrelationValuePath is a JSONPath expression to extract the correlation value from the input
	// If not specified, the entire input is used as the correlation value
	CorrelationValuePath *string `json:"CorrelationValuePath,omitempty"`

	// TimeoutSeconds specifies how long to wait for a message before timing out
	// If not specified, waits indefinitely
	TimeoutSeconds *int `json:"TimeoutSeconds,omitempty"`

	// TimeoutPath specifies the next state to transition to on timeout
	// If not specified, the execution fails on timeout
	TimeoutPath *string `json:"TimeoutPath,omitempty"`

	// MessagePath specifies where to place the received message data in the state output
	// Default is "$" (replace entire output with message data)
	MessagePath *string `json:"MessagePath,omitempty"`

	// Retry rules for the message state
	Retry []RetryRule `json:"Retry,omitempty"`

	// Catch rules for handling errors
	Catch []CatchRule `json:"Catch,omitempty"`
}

// MessageStateData holds the correlation data created during pre-message phase
type MessageStateData struct {
	ExecutionID      string      `json:"execution_id"`
	StateName        string      `json:"state_name"`
	CorrelationKey   string      `json:"correlation_key"`
	CorrelationValue interface{} `json:"correlation_value"`
	InputData        interface{} `json:"input_data"`
	CreatedAt        int64       `json:"created_at"`           // Unix timestamp
	TimeoutAt        *int64      `json:"timeout_at,omitempty"` // Unix timestamp
}

// MessageData represents the incoming message
type MessageData struct {
	CorrelationKey   string      `json:"correlation_key"`
	CorrelationValue interface{} `json:"correlation_value"`
	Data             interface{} `json:"data"`
}

// MessageStateResult represents the result of a message state execution
type MessageStateResult struct {
	Status          string            `json:"status"` // "WAITING", "RECEIVED", "TIMEOUT"
	CorrelationData *MessageStateData `json:"correlation_data,omitempty"`
	ReceivedMessage *MessageData      `json:"received_message,omitempty"`
}

// TimeoutScheduleRequest represents a request to schedule a timeout execution
type TimeoutScheduleRequest struct {
	ExecutionID    string `json:"execution_id"`
	StateMachineID string `json:"state_machine_id"`
	StateName      string `json:"state_name"`
	CorrelationID  string `json:"correlation_id"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	ScheduleTime   int64  `json:"schedule_time"` // Unix timestamp when to trigger
}

// NewMessageState creates a new message state
func NewMessageState(name, correlationKey string) *MessageState {
	return &MessageState{
		BaseState: BaseState{
			Name: name,
			Type: "Message",
		},
		CorrelationKey: correlationKey,
	}
}

// Execute implements the State interface
// For pre-message phase: creates correlation data and returns WAITING status
// For post-message phase: processes received message and continues
func (s *MessageState) Execute(ctx context.Context, input interface{}) (result interface{}, nextState *string, err error) {
	processor := GetPathProcessor()

	// Apply InputPath if specified
	effectiveInput := input
	if s.InputPath != nil {
		processedInput, err := processor.ApplyInputPath(input, s.InputPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to apply InputPath: %w", err)
		}
		effectiveInput = processedInput
	}

	// Check if this is a timeout resumption
	if _, isTimeout := s.checkForTimeout(effectiveInput); isTimeout {
		return s.executeTimeout(ctx, effectiveInput, processor)
	}

	// Check if this is a resume operation (post-message)
	if messageData, isResume := s.checkForReceivedMessage(effectiveInput); isResume {
		return s.executePostMessage(ctx, input, effectiveInput, messageData, processor)
	}

	// Pre-message phase: create correlation data and pause
	return s.executePreMessage(ctx, input, effectiveInput, processor)
}

// executePreMessage handles the pre-message phase
func (s *MessageState) executePreMessage(_ context.Context, originalInput, effectiveInput interface{}, processor PathProcessor) (result interface{}, nextState *string, err error) {
	// Extract correlation value from input
	correlationValue := effectiveInput
	if s.CorrelationValuePath != nil {
		var err error
		correlationValue, err = processor.ApplyInputPath(effectiveInput, s.CorrelationValuePath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to extract correlation value: %w", err)
		}
	}

	// Calculate timeout timestamp if specified
	var timeoutAt *int64
	if s.TimeoutSeconds != nil && *s.TimeoutSeconds > 0 {
		timeout := time.Now().Unix() + int64(*s.TimeoutSeconds)
		timeoutAt = &timeout
	}

	// Create the result with correlation data
	messageResult := &MessageStateResult{
		Status: "WAITING",
		CorrelationData: &MessageStateData{
			CorrelationKey:   s.CorrelationKey,
			CorrelationValue: correlationValue,
			InputData:        effectiveInput,
			CreatedAt:        time.Now().Unix(),
			TimeoutAt:        timeoutAt,
		},
	}

	// Apply ResultPath
	var finalResult interface{}
	if s.ResultPath != nil {
		var err error
		finalResult, err = processor.ApplyResultPath(originalInput, messageResult, s.ResultPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to apply ResultPath: %w", err)
		}
	} else {
		finalResult = messageResult
	}

	// Apply OutputPath
	if s.OutputPath != nil {
		var err error
		finalResult, err = processor.ApplyOutputPath(finalResult, s.OutputPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to apply OutputPath: %w", err)
		}
	}

	// Return nil for next state to signal pause
	return finalResult, nil, nil
}

// executePostMessage handles the post-message phase (after message is received)
func (s *MessageState) executePostMessage(_ context.Context, originalInput, effectiveInput interface{}, messageData *MessageData, processor PathProcessor) (result interface{}, nextState *string, err error) {
	// Process the received message data
	messageResult := messageData.Data

	// If MessagePath is specified, place the message at that path
	if s.MessagePath != nil && *s.MessagePath != "$" {
		// Merge message data into the original input at the specified path
		messageResult = effectiveInput
		// TODO: Implement path-based merging when JSONPath processor supports it
	}

	// Apply ResultPath
	var finalResult interface{}
	if s.ResultPath != nil {
		var err error
		finalResult, err = processor.ApplyResultPath(originalInput, messageResult, s.ResultPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to apply ResultPath: %w", err)
		}
	} else {
		finalResult = messageResult
	}

	// Apply OutputPath
	if s.OutputPath != nil {
		var err error
		finalResult, err = processor.ApplyOutputPath(finalResult, s.OutputPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to apply OutputPath: %w", err)
		}
	}

	// Continue to next state (normal path)
	return finalResult, s.Next, nil
}

// executeTimeout handles the timeout scenario
func (s *MessageState) executeTimeout(_ context.Context, originalInput interface{}, processor PathProcessor) (result interface{}, nextState *string, err error) {
	// Create timeout result
	timeoutResult := map[string]interface{}{
		"status":  "TIMEOUT",
		"message": fmt.Sprintf("Message state '%s' timed out waiting for correlation", s.Name),
	}

	// Apply ResultPath
	var finalResult interface{}
	if s.ResultPath != nil {
		var err error
		finalResult, err = processor.ApplyResultPath(originalInput, timeoutResult, s.ResultPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to apply ResultPath: %w", err)
		}
	} else {
		finalResult = timeoutResult
	}

	// Apply OutputPath
	if s.OutputPath != nil {
		var err error
		finalResult, err = processor.ApplyOutputPath(finalResult, s.OutputPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to apply OutputPath: %w", err)
		}
	}

	// If TimeoutPath is specified, transition to that state
	// Otherwise, this will be treated as an error by the state machine
	if s.TimeoutPath != nil {
		return finalResult, s.TimeoutPath, nil
	}

	// No timeout path specified - return error
	return finalResult, nil, fmt.Errorf("message state '%s' timed out", s.Name)
}

// checkForReceivedMessage checks if the input contains a received message
// This is used to determine if we're in post-message phase
func (s *MessageState) checkForReceivedMessage(input interface{}) (messageData *MessageData, isResume bool) {
	// Check if input is a map containing message metadata
	if inputMap, ok := input.(map[string]interface{}); ok {
		receivedMessageKey := fmt.Sprintf("%s_%s", ReceivedMessageBase, s.Name)
		if msgData, exists := inputMap[receivedMessageKey]; exists {
			// Try to parse as MessageData
			if msgMap, ok := msgData.(map[string]interface{}); ok {
				messageData = &MessageData{
					CorrelationKey: s.CorrelationKey,
				}

				if key, ok := msgMap["correlation_key"].(string); ok {
					messageData.CorrelationKey = key
				}
				if val, exists := msgMap["correlation_value"]; exists {
					messageData.CorrelationValue = val
				}
				if data, exists := msgMap["data"]; exists {
					messageData.Data = data
				}

				return messageData, true
			}
		}
	}

	return nil, false
}

// checkForTimeout checks if the input contains a timeout trigger
// This is used to determine if we're executing the timeout boundary event
func (s *MessageState) checkForTimeout(input interface{}) (timeoutData map[string]interface{}, isTimeout bool) {
	// Check if input is a map containing timeout metadata

	triggerKey := fmt.Sprintf("%s_%s", types.TriggerTimeoutBase, s.Name)

	if inputMap, ok := input.(map[string]interface{}); ok {
		if _, exists := inputMap[triggerKey]; exists {
			return inputMap, true
		}
	}

	return nil, false
}

// Validate validates the message state configuration
func (s *MessageState) Validate() (err error) {
	if err := s.BaseState.Validate(); err != nil {
		return err
	}

	if s.CorrelationKey == "" {
		return fmt.Errorf("message state '%s': CorrelationKey cannot be empty", s.Name)
	}

	if s.TimeoutSeconds != nil && *s.TimeoutSeconds < 0 {
		return fmt.Errorf("message state '%s': TimeoutSeconds cannot be negative", s.Name)
	}

	// Validate that if TimeoutSeconds is set, either TimeoutPath or error handling is configured
	if s.TimeoutSeconds != nil && *s.TimeoutSeconds > 0 {
		if s.TimeoutPath == nil && len(s.Catch) == 0 {
			return fmt.Errorf("message state '%s': TimeoutSeconds is set but no TimeoutPath or Catch handlers configured", s.Name)
		}
	}

	// Validate retry rules
	for i, retry := range s.Retry {
		if len(retry.ErrorEquals) == 0 {
			return fmt.Errorf("message state '%s': Retry[%d] must have at least one ErrorEquals", s.Name, i)
		}
	}

	// Validate catch rules
	for i, catch := range s.Catch {
		if len(catch.ErrorEquals) == 0 {
			return fmt.Errorf("message state '%s': Catch[%d] must have at least one ErrorEquals", s.Name, i)
		}
		if catch.Next == "" {
			return fmt.Errorf("message state '%s': Catch[%d] must have a Next state", s.Name, i)
		}
	}

	return nil
}

// MarshalJSON implements custom JSON marshaling for MessageState.
func (s *MessageState) MarshalJSON() (data []byte, err error) {
	// Define only the custom fields (not BaseState!)
	customFields := struct {
		CorrelationKey       string      `json:"CorrelationKey"`
		CorrelationValuePath *string     `json:"CorrelationValuePath,omitempty"`
		TimeoutSeconds       *int        `json:"TimeoutSeconds,omitempty"`
		TimeoutPath          *string     `json:"TimeoutPath,omitempty"`
		MessagePath          *string     `json:"MessagePath,omitempty"`
		Retry                []RetryRule `json:"Retry,omitempty"`
		Catch                []CatchRule `json:"Catch,omitempty"`
	}{
		CorrelationKey:       s.CorrelationKey,
		CorrelationValuePath: s.CorrelationValuePath,
		TimeoutSeconds:       s.TimeoutSeconds,
		TimeoutPath:          s.TimeoutPath,
		MessagePath:          s.MessagePath,
		Retry:                s.Retry,
		Catch:                s.Catch,
	}

	return MarshalStateWithBase(s.BaseState, customFields)
}

// GetNextStates returns all possible next states
func (s *MessageState) GetNextStates() (nextStates []string) {
	nextStates = s.BaseState.GetNextStates()

	// Add timeout path
	if s.TimeoutPath != nil {
		nextStates = append(nextStates, *s.TimeoutPath)
	}

	// Add catch destinations
	for _, catch := range s.Catch {
		nextStates = append(nextStates, catch.Next)
	}

	if s.GetTimeoutPath() != nil {
		nextStates = append(nextStates, *s.GetTimeoutPath())
	}

	return nextStates
}

// GetCorrelationKey returns the correlation key
func (s *MessageState) GetCorrelationKey() (key string) {
	return s.CorrelationKey
}

// GetTimeoutSeconds returns the timeout in seconds
func (s *MessageState) GetTimeoutSeconds() (timeout *int) {
	return s.TimeoutSeconds
}

// GetTimeoutPath returns the timeout path
func (s *MessageState) GetTimeoutPath() (path *string) {
	return s.TimeoutPath
}

// IsWaitingState returns true to indicate this is a waiting state
func (s *MessageState) IsWaitingState() (isWaiting bool) {
	return true
}

// CreateTimeoutScheduleRequest creates a request for scheduling a timeout execution
func (s *MessageState) CreateTimeoutScheduleRequest(executionID, stateMachineID, correlationID string) *TimeoutScheduleRequest {
	if s.TimeoutSeconds == nil || *s.TimeoutSeconds <= 0 {
		return nil
	}

	scheduleTime := time.Now().Unix() + int64(*s.TimeoutSeconds)

	return &TimeoutScheduleRequest{
		ExecutionID:    executionID,
		StateMachineID: stateMachineID,
		StateName:      s.Name,
		CorrelationID:  correlationID,
		TimeoutSeconds: *s.TimeoutSeconds,
		ScheduleTime:   scheduleTime,
	}
}
