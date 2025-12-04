package errors

import (
	"fmt"
)

// StateMachineError represents an error in state machine execution
type StateMachineError struct {
	ErrorType string
	Message   string
	Cause     error
}

func (e *StateMachineError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (cause: %v)", e.ErrorType, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.ErrorType, e.Message)
}

// GetErrorType returns the error type
func (e *StateMachineError) GetErrorType() string {
	return e.ErrorType
}

// NewStateMachineError creates a new StateMachineError
func NewStateMachineError(errorType, message string, cause error) *StateMachineError {
	return &StateMachineError{
		ErrorType: errorType,
		Message:   message,
		Cause:     cause,
	}
}

// IsTimeoutError checks if an error is a timeout error
func IsTimeoutError(err error) bool {
	if smErr, ok := err.(*StateMachineError); ok {
		return smErr.ErrorType == "States.Timeout"
	}
	return false
}

// IsTaskFailedError checks if an error is a task failure error
func IsTaskFailedError(err error) bool {
	if smErr, ok := err.(*StateMachineError); ok {
		return smErr.ErrorType == "States.TaskFailed"
	}
	return false
}

// Error mapping for common errors
var ErrorMappings = map[string]string{
	"States.Timeout":                         "A Task State either ran longer than the TimeoutSeconds value, or failed to heartbeat for a time longer than the HeartbeatSeconds value.",
	"States.TaskFailed":                      "A Task State failed during the execution.",
	"States.Permissions":                     "A Task State failed because it had insufficient privileges to execute the specified code.",
	"States.ResultPathMatchFailure":          "A state's ResultPath field cannot be applied to the input the state received.",
	"States.ParameterPathFailure":            "Within a state's Parameters field, the attempt to replace a field whose name ends in .$ using a Path failed.",
	"States.BranchFailed":                    "A branch of a Parallel State failed.",
	"States.NoChoiceMatched":                 "A Choice State failed to find a match for any condition.",
	"States.IntrinsicFailure":                "Within a Payload Template, the attempt to invoke an Intrinsic Function failed.",
	"States.ExceedToleratedFailureThreshold": "A Map State failed because the number of failed items exceeded the tolerated failure threshold.",
	"States.ItemReaderFailed":                "A Map State failed because it was unable to read all the items from the dataset specified in the ItemsPath field.",
	"States.ResultWriterFailed":              "A Map State failed because it was unable to write all the result items to the dataset specified in the ResultPath field.",
}

// Common error constructors
func NewTimeoutError(message string, cause error) *StateMachineError {
	return NewStateMachineError("States.Timeout", message, cause)
}

func NewTaskFailedError(message string, cause error) *StateMachineError {
	return NewStateMachineError("States.TaskFailed", message, cause)
}

func NewParameterPathFailureError(message string, cause error) *StateMachineError {
	return NewStateMachineError("States.ParameterPathFailure", message, cause)
}

func NewResultPathMatchFailureError(message string, cause error) *StateMachineError {
	return NewStateMachineError("States.ResultPathMatchFailure", message, cause)
}

func NewBranchFailedError(message string, cause error) *StateMachineError {
	return NewStateMachineError("States.BranchFailed", message, cause)
}

func NewNoChoiceMatchedError(message string, cause error) *StateMachineError {
	return NewStateMachineError("States.NoChoiceMatched", message, cause)
}

func NewIntrinsicFailureError(message string, cause error) *StateMachineError {
	return NewStateMachineError("States.IntrinsicFailure", message, cause)
}

func NewPermissionsError(message string, cause error) *StateMachineError {
	return NewStateMachineError("States.Permissions", message, cause)
}

// Error check helpers
func IsErrorType(err error, errorType string) bool {
	if smErr, ok := err.(*StateMachineError); ok {
		return smErr.ErrorType == errorType
	}
	return false
}

// WrapError wraps a generic error as a StateMachineError
func WrapError(errorType string, err error) *StateMachineError {
	if err == nil {
		return nil
	}

	if smErr, ok := err.(*StateMachineError); ok {
		return smErr
	}

	return NewStateMachineError(errorType, err.Error(), err)
}

// GetErrorMessage returns the error message from ErrorMappings if available
func GetErrorMessage(errorType string) string {
	if msg, ok := ErrorMappings[errorType]; ok {
		return msg
	}
	return "An unknown error occurred"
}

// Helper to check if error is retryable
func IsRetryableError(err error, errorEquals []string) bool {
	errorType := ""
	if smErr, ok := err.(*StateMachineError); ok {
		errorType = smErr.ErrorType
	}

	for _, eq := range errorEquals {
		if eq == errorType || eq == "States.ALL" {
			return true
		}
	}
	return false
}
