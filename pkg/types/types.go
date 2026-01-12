package types

import (
	"context"
)

// State types as defined in AWS States Language
const (
	StateTypeTask     = "Task"
	StateTypePass     = "Pass"
	StateTypeChoice   = "Choice"
	StateTypeWait     = "Wait"
	StateTypeSucceed  = "Succeed"
	StateTypeFail     = "Fail"
	StateTypeParallel = "Parallel"
)

// Error types
const (
	ErrorTypeStatesTimeout                         = "States.Timeout"
	ErrorTypeStatesTaskFailed                      = "States.TaskFailed"
	ErrorTypeStatesPermissions                     = "States.Permissions"
	ErrorTypeStatesResultPathMatchFailure          = "States.ResultPathMatchFailure"
	ErrorTypeStatesParameterPathFailure            = "States.ParameterPathFailure"
	ErrorTypeStatesBranchFailed                    = "States.BranchFailed"
	ErrorTypeStatesNoChoiceMatched                 = "States.NoChoiceMatched"
	ErrorTypeStatesIntrinsicFailure                = "States.IntrinsicFailure"
	ErrorTypeStatesExceedToleratedFailureThreshold = "States.ExceedToleratedFailureThreshold"
	ErrorTypeStatesItemReaderFailed                = "States.ItemReaderFailed"
	ErrorTypeStatesResultWriterFailed              = "States.ResultWriterFailed"
)

// Common field names
const (
	FieldType           = "Type"
	FieldNext           = "Next"
	FieldEnd            = "End"
	FieldResource       = "Resource"
	FieldResult         = "Result"
	FieldResultPath     = "ResultPath"
	FieldInputPath      = "InputPath"
	FieldOutputPath     = "OutputPath"
	FieldParameters     = "Parameters"
	FieldRetry          = "Retry"
	FieldCatch          = "Catch"
	FieldChoices        = "Choices"
	FieldDefault        = "Default"
	FieldSeconds        = "Seconds"
	FieldTimestamp      = "Timestamp"
	FieldSecondsPath    = "SecondsPath"
	FieldTimestampPath  = "TimestampPath"
	FieldComment        = "Comment"
	FieldCause          = "Cause"
	FieldError          = "Error"
	FieldBranches       = "Branches"
	FieldIterator       = "Iterator"
	FieldItemsPath      = "ItemsPath"
	FieldMaxConcurrency = "MaxConcurrency"
)

// Execution statuses
const (
	StatusRunning   = "RUNNING"
	StatusSucceeded = "SUCCEEDED"
	StatusFailed    = "FAILED"
	StatusTimedOut  = "TIMED_OUT"
	StatusAborted   = "ABORTED"
)

// Default timeout values
const (
	DefaultTimeoutSeconds = 3600  // 1 hour
	MaxTimeoutSeconds     = 86400 // 24 hours
)

// Context keys for task execution
type contextKey string

const (
	// ExecutionContextKey is the key for storing execution context in context
	ExecutionContextKey contextKey = "execution_context"
)

// Version constants
const (
	Version1 = "1.0"
)

// ExecutionContext provides access to execution-related functionality
type ExecutionContext interface {
	// GetTaskHandler retrieves a task handler for a resource
	GetTaskHandler(resource string) (func(context.Context, interface{}) (interface{}, error), bool)
}
