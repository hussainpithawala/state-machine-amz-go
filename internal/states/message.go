package states

import (
	"context"
	"encoding/json"
	"fmt"
)

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

// NewMessageState creates a new message state
func NewMessageState(name string, correlationKey string) *MessageState {
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
func (s *MessageState) Execute(ctx context.Context, input interface{}) (interface{}, *string, error) {
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

	// Check if this is a resume operation (post-message)
	// This is determined by checking if the input contains a received message
	if messageData, isResume := s.checkForReceivedMessage(effectiveInput); isResume {
		return s.executePostMessage(ctx, input, effectiveInput, messageData, processor)
	}

	// Pre-message phase: create correlation data and pause
	return s.executePreMessage(ctx, input, effectiveInput, processor)
}

// executePreMessage handles the pre-message phase
func (s *MessageState) executePreMessage(ctx context.Context, originalInput, effectiveInput interface{}, processor PathProcessor) (interface{}, *string, error) {
	// Extract correlation value from input
	correlationValue := effectiveInput
	if s.CorrelationValuePath != nil {
		var err error
		correlationValue, err = processor.ApplyInputPath(effectiveInput, s.CorrelationValuePath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to extract correlation value: %w", err)
		}
	}

	// Create the result with correlation data
	result := &MessageStateResult{
		Status: "WAITING",
		CorrelationData: &MessageStateData{
			CorrelationKey:   s.CorrelationKey,
			CorrelationValue: correlationValue,
			InputData:        effectiveInput,
		},
	}

	// Apply ResultPath
	var finalResult interface{}
	if s.ResultPath != nil {
		var err error
		finalResult, err = processor.ApplyResultPath(originalInput, result, s.ResultPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to apply ResultPath: %w", err)
		}
	} else {
		finalResult = result
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
func (s *MessageState) executePostMessage(ctx context.Context, originalInput, effectiveInput interface{}, messageData *MessageData, processor PathProcessor) (interface{}, *string, error) {
	// Process the received message data
	result := messageData.Data

	// If MessagePath is specified, place the message at that path
	if s.MessagePath != nil && *s.MessagePath != "$" {
		// Merge message data into the original input at the specified path
		result = effectiveInput
		// TODO: Implement path-based merging when JSONPath processor supports it
	}

	// Apply ResultPath
	var finalResult interface{}
	if s.ResultPath != nil {
		var err error
		finalResult, err = processor.ApplyResultPath(originalInput, result, s.ResultPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to apply ResultPath: %w", err)
		}
	} else {
		finalResult = result
	}

	// Apply OutputPath
	if s.OutputPath != nil {
		var err error
		finalResult, err = processor.ApplyOutputPath(finalResult, s.OutputPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to apply OutputPath: %w", err)
		}
	}

	// Continue to next state
	return finalResult, s.Next, nil
}

// checkForReceivedMessage checks if the input contains a received message
// This is used to determine if we're in post-message phase
func (s *MessageState) checkForReceivedMessage(input interface{}) (*MessageData, bool) {
	// Check if input is a map containing message metadata
	if inputMap, ok := input.(map[string]interface{}); ok {
		if msgData, exists := inputMap["__received_message__"]; exists {
			// Try to parse as MessageData
			if msgMap, ok := msgData.(map[string]interface{}); ok {
				messageData := &MessageData{
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

// Validate validates the message state configuration
func (s *MessageState) Validate() error {
	if err := s.BaseState.Validate(); err != nil {
		return err
	}

	if s.CorrelationKey == "" {
		return fmt.Errorf("message state '%s': CorrelationKey cannot be empty", s.Name)
	}

	if s.TimeoutSeconds != nil && *s.TimeoutSeconds < 0 {
		return fmt.Errorf("message state '%s': TimeoutSeconds cannot be negative", s.Name)
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

// MarshalJSON implements custom JSON marshaling
func (s *MessageState) MarshalJSON() ([]byte, error) {
	type Alias MessageState
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(s),
	})
}

// GetNextStates returns all possible next states
func (s *MessageState) GetNextStates() []string {
	nextStates := s.BaseState.GetNextStates()

	// Add catch destinations
	for _, catch := range s.Catch {
		nextStates = append(nextStates, catch.Next)
	}

	return nextStates
}

// GetCorrelationKey returns the correlation key
func (s *MessageState) GetCorrelationKey() string {
	return s.CorrelationKey
}

// GetTimeoutSeconds returns the timeout in seconds
func (s *MessageState) GetTimeoutSeconds() *int {
	return s.TimeoutSeconds
}

// IsWaitingState returns true to indicate this is a waiting state
func (s *MessageState) IsWaitingState() bool {
	return true
}
