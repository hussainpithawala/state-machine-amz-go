// In choice.go, replace the entire file with this fixed version:

package states

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// ChoiceState represents a Choice state for conditional branching
type ChoiceState struct {
	BaseState
	Choices []ChoiceRule `json:"Choices"`
	Default *string      `json:"Default,omitempty"`
}

// ChoiceRule represents a single choice rule
type ChoiceRule struct {
	Variable                   string       `json:"Variable"`
	StringEquals               *string      `json:"StringEquals,omitempty"`
	StringLessThan             *string      `json:"StringLessThan,omitempty"`
	StringGreaterThan          *string      `json:"StringGreaterThan,omitempty"`
	StringLessThanEquals       *string      `json:"StringLessThanEquals,omitempty"`
	StringGreaterThanEquals    *string      `json:"StringGreaterThanEquals,omitempty"`
	NumericEquals              *float64     `json:"NumericEquals,omitempty"`
	NumericLessThan            *float64     `json:"NumericLessThan,omitempty"`
	NumericGreaterThan         *float64     `json:"NumericGreaterThan,omitempty"`
	NumericLessThanEquals      *float64     `json:"NumericLessThanEquals,omitempty"`
	NumericGreaterThanEquals   *float64     `json:"NumericGreaterThanEquals,omitempty"`
	BooleanEquals              *bool        `json:"BooleanEquals,omitempty"`
	TimestampEquals            *string      `json:"TimestampEquals,omitempty"`
	TimestampLessThan          *string      `json:"TimestampLessThan,omitempty"`
	TimestampGreaterThan       *string      `json:"TimestampGreaterThan,omitempty"`
	TimestampLessThanEquals    *string      `json:"TimestampLessThanEquals,omitempty"`
	TimestampGreaterThanEquals *string      `json:"TimestampGreaterThanEquals,omitempty"`
	And                        []ChoiceRule `json:"And,omitempty"`
	Or                         []ChoiceRule `json:"Or,omitempty"`
	Not                        *ChoiceRule  `json:"Not,omitempty"`
	Next                       string       `json:"Next"`
	Comment                    string       `json:"Comment,omitempty"`
}

// Execute executes the Choice state
func (s *ChoiceState) Execute(ctx context.Context, input interface{}) (interface{}, *string, error) {
	// Process input path
	processor := GetPathProcessor()
	processedInput, err := processor.ApplyInputPath(input, s.InputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to process input path: %w", err)
	}

	// Evaluate each choice
	for index := range s.Choices {
		choice := &s.Choices[index]
		matched, err := s.evaluateChoice(choice, processedInput)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to evaluate choice: %w", err)
		}

		if matched {
			// Apply result path and output path
			processedResult, err := processor.ApplyResultPath(processedInput, processedInput, s.ResultPath)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to process result path: %w", err)
			}

			finalOutput, err := processor.ApplyOutputPath(processedResult, s.OutputPath)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to process output path: %w", err)
			}

			return finalOutput, &choice.Next, nil
		}
	}

	// No choice matched, use default if specified
	if s.Default != nil {
		// Apply result path and output path
		processedResult, err := processor.ApplyResultPath(processedInput, processedInput, s.ResultPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to process result path: %w", err)
		}

		finalOutput, err := processor.ApplyOutputPath(processedResult, s.OutputPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to process output path: %w", err)
		}

		return finalOutput, s.Default, nil
	}

	// No choice matched and no default - this is an error
	return nil, nil, fmt.Errorf("no choice rule matched and no default specified")
}

// evaluateChoice evaluates a single choice rule
func (s *ChoiceState) evaluateChoice(rule *ChoiceRule, input interface{}) (bool, error) {
	// Determine the context for this rule
	var context interface{} = input
	if rule.Variable != "" {
		context = s.getVariableValue(rule.Variable, input)
		// If the variable doesn't exist, the choice evaluates to false
		if context == nil {
			return false, nil
		}
	}

	// Handle compound operators
	if len(rule.And) > 0 {
		return s.evaluateAnd(rule.And, context)
	}

	if len(rule.Or) > 0 {
		return s.evaluateOr(rule.Or, context)
	}

	if rule.Not != nil {
		matched, err := s.evaluateChoice(rule.Not, context)
		if err != nil {
			return false, err
		}
		return !matched, nil
	}

	// If we get here, we have comparison operators
	// Evaluate based on comparison operators
	return s.evaluateComparison(rule, context)
}

// evaluateAnd evaluates AND conditions
func (s *ChoiceState) evaluateAnd(rules []ChoiceRule, context interface{}) (bool, error) {
	for index := range rules {
		rule := &rules[index]
		matched, err := s.evaluateChoice(rule, context)
		if err != nil {
			return false, err
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}

// evaluateOr evaluates OR conditions
func (s *ChoiceState) evaluateOr(rules []ChoiceRule, context interface{}) (bool, error) {
	for index := range rules {
		rule := &rules[index]
		matched, err := s.evaluateChoice(rule, context)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

// evaluateComparison evaluates comparison operators
func (s *ChoiceState) evaluateComparison(rule *ChoiceRule, variableValue interface{}) (bool, error) {
	// Check each comparison operator
	if rule.StringEquals != nil {
		return s.compareString(variableValue, *rule.StringEquals, func(a, b string) bool { return a == b })
	}
	if rule.StringLessThan != nil {
		return s.compareString(variableValue, *rule.StringLessThan, func(a, b string) bool { return a < b })
	}
	if rule.StringGreaterThan != nil {
		return s.compareString(variableValue, *rule.StringGreaterThan, func(a, b string) bool { return a > b })
	}
	if rule.StringLessThanEquals != nil {
		return s.compareString(variableValue, *rule.StringLessThanEquals, func(a, b string) bool { return a <= b })
	}
	if rule.StringGreaterThanEquals != nil {
		return s.compareString(variableValue, *rule.StringGreaterThanEquals, func(a, b string) bool { return a >= b })
	}
	if rule.NumericEquals != nil {
		return s.compareNumeric(variableValue, *rule.NumericEquals, func(a, b float64) bool { return a == b })
	}
	if rule.NumericLessThan != nil {
		return s.compareNumeric(variableValue, *rule.NumericLessThan, func(a, b float64) bool { return a < b })
	}
	if rule.NumericGreaterThan != nil {
		return s.compareNumeric(variableValue, *rule.NumericGreaterThan, func(a, b float64) bool { return a > b })
	}
	if rule.NumericLessThanEquals != nil {
		return s.compareNumeric(variableValue, *rule.NumericLessThanEquals, func(a, b float64) bool { return a <= b })
	}
	if rule.NumericGreaterThanEquals != nil {
		return s.compareNumeric(variableValue, *rule.NumericGreaterThanEquals, func(a, b float64) bool { return a >= b })
	}
	if rule.BooleanEquals != nil {
		return s.compareBoolean(variableValue, *rule.BooleanEquals)
	}
	if rule.TimestampEquals != nil {
		return s.compareTimestamp(variableValue, *rule.TimestampEquals, func(a, b time.Time) bool { return a.Equal(b) })
	}
	if rule.TimestampLessThan != nil {
		return s.compareTimestamp(variableValue, *rule.TimestampLessThan, func(a, b time.Time) bool { return a.Before(b) })
	}
	if rule.TimestampGreaterThan != nil {
		return s.compareTimestamp(variableValue, *rule.TimestampGreaterThan, func(a, b time.Time) bool { return a.After(b) })
	}
	if rule.TimestampLessThanEquals != nil {
		return s.compareTimestamp(variableValue, *rule.TimestampLessThanEquals, func(a, b time.Time) bool { return a.Before(b) || a.Equal(b) })
	}
	if rule.TimestampGreaterThanEquals != nil {
		return s.compareTimestamp(variableValue, *rule.TimestampGreaterThanEquals, func(a, b time.Time) bool { return a.After(b) || a.Equal(b) })
	}

	return false, fmt.Errorf("no comparison operator specified in choice rule")
}

// compareString compares string values
func (s *ChoiceState) compareString(variableValue interface{}, expected string, compareFunc func(a, b string) bool) (bool, error) {
	strValue, ok := variableValue.(string)
	if !ok {
		// Try to convert to string
		strValue = fmt.Sprintf("%v", variableValue)
	}

	return compareFunc(strValue, expected), nil
}

// compareNumeric compares numeric values
func (s *ChoiceState) compareNumeric(variableValue interface{}, expected float64, compareFunc func(a, b float64) bool) (bool, error) {
	// Try to convert variableValue to float64
	var floatValue float64

	switch v := variableValue.(type) {
	case float64:
		floatValue = v
	case float32:
		floatValue = float64(v)
	case int:
		floatValue = float64(v)
	case int32:
		floatValue = float64(v)
	case int64:
		floatValue = float64(v)
	case uint:
		floatValue = float64(v)
	case uint32:
		floatValue = float64(v)
	case uint64:
		floatValue = float64(v)
	case string:
		// Try to parse string as float
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return false, fmt.Errorf("cannot convert string '%s' to number: %w", v, err)
		}
		floatValue = parsed
	default:
		return false, fmt.Errorf("cannot compare non-numeric value %v with numeric operator", variableValue)
	}

	return compareFunc(floatValue, expected), nil
}

// compareBoolean compares boolean values
func (s *ChoiceState) compareBoolean(variableValue interface{}, expected bool) (bool, error) {
	boolValue, ok := variableValue.(bool)
	if !ok {
		// Try to convert from string
		if strValue, ok := variableValue.(string); ok {
			lowerStr := strings.ToLower(strValue)
			switch {
			case lowerStr == "true":
				boolValue = true
			case lowerStr == "false":
				boolValue = false
			default:
				return false, fmt.Errorf("cannot convert string '%s' to boolean", strValue)
			}
		} else {
			return false, fmt.Errorf("cannot compare non-boolean value %v with boolean operator", variableValue)
		}
	}

	return boolValue == expected, nil
}

// compareTimestamp compares timestamp values
func (s *ChoiceState) compareTimestamp(variableValue interface{}, expectedStr string, compareFunc func(a, b time.Time) bool) (bool, error) {
	// Parse expected timestamp
	expectedTime, err := time.Parse(time.RFC3339, expectedStr)
	if err != nil {
		// Try other common formats
		formats := []string{
			"2006-01-02T15:04:05Z",
			"2006-01-02T15:04:05-07:00",
			"2006-01-02 15:04:05",
			time.RFC1123,
			time.RFC822,
		}

		for _, format := range formats {
			expectedTime, err = time.Parse(format, expectedStr)
			if err == nil {
				break
			}
		}

		if err != nil {
			return false, fmt.Errorf("invalid timestamp format '%s'", expectedStr)
		}
	}

	// Get variable timestamp
	var variableTime time.Time

	switch v := variableValue.(type) {
	case time.Time:
		variableTime = v
	case string:
		// Try to parse as timestamp
		variableTime, err = time.Parse(time.RFC3339, v)
		if err != nil {
			// Try other formats
			for _, format := range []string{
				"2006-01-02T15:04:05Z",
				"2006-01-02T15:04:05-07:00",
				"2006-01-02 15:04:05",
				time.RFC1123,
				time.RFC822,
			} {
				variableTime, err = time.Parse(format, v)
				if err == nil {
					break
				}
			}

			if err != nil {
				return false, fmt.Errorf("cannot convert string '%s' to timestamp: %w", v, err)
			}
		}
	case float64:
		// Assume Unix timestamp (seconds since epoch)
		variableTime = time.Unix(int64(v), 0)
	case int:
		// Assume Unix timestamp
		variableTime = time.Unix(int64(v), 0)
	case int64:
		// Assume Unix timestamp
		variableTime = time.Unix(v, 0)
	default:
		return false, fmt.Errorf("cannot compare non-timestamp value %v with timestamp operator", variableValue)
	}

	return compareFunc(variableTime, expectedTime), nil
}

// getVariableValue gets a value from input at a JSONPath
func (s *ChoiceState) getVariableValue(path string, input interface{}) interface{} {
	processor := GetPathProcessor()

	// Use ApplyInputPath to extract the value at the path
	value, err := processor.ApplyInputPath(input, &path)
	if err != nil {
		// Any error accessing the variable means it doesn't exist or is inaccessible
		// According to AWS Step Functions, this results in the choice evaluating to false
		return nil
	}

	return value
}

// Validate validates the Choice state configuration
func (s *ChoiceState) Validate() error {
	// Don't call BaseState.Validate() because Choice states don't need Next or End
	// Instead, validate the basic fields
	if s.Name == "" {
		return fmt.Errorf("state name cannot be empty")
	}

	if s.Type != "Choice" {
		return fmt.Errorf("choice state must have Type 'Choice'")
	}

	// Choice states cannot have Next (choices have their own Next)
	if s.Next != nil {
		return fmt.Errorf("choice state '%s' cannot have Next field", s.Name)
	}

	// Choice states cannot have End
	if s.End {
		return fmt.Errorf("choice state '%s' cannot have End field", s.Name)
	}

	// Must have at least one choice or a default
	if len(s.Choices) == 0 && s.Default == nil {
		return fmt.Errorf("choice state '%s' must have either Choices or Default", s.Name)
	}

	// Validate each choice
	for index := range s.Choices {
		if err := s.validateChoice(&s.Choices[index], index, true); err != nil {
			return fmt.Errorf("choice %d: %w", index, err)
		}
	}
	return nil
}

// validateChoice validates a single choice rule
func (s *ChoiceState) validateChoice(choice *ChoiceRule, index int, nextRequired bool) error {
	// Count operators
	comparisonCount := countComparisonOperators(choice)
	compoundCount := countCompoundOperators(choice)

	// Validate operator requirements
	if err := validateOperatorRequirements(choice, index, comparisonCount, compoundCount); err != nil {
		return err
	}

	// Validate Next field if required
	if nextRequired && choice.Next == "" {
		return fmt.Errorf("choice %d: Next is required", index)
	}

	// Validate nested compound operators recursively
	return validateNestedChoices(choice, index)
}

// Helper function to count comparison operators
func countComparisonOperators(choice *ChoiceRule) int {
	type operatorCheck struct {
		field interface{}
	}

	operators := []operatorCheck{
		{choice.StringEquals},
		{choice.StringLessThan},
		{choice.StringGreaterThan},
		{choice.StringLessThanEquals},
		{choice.StringGreaterThanEquals},
		{choice.NumericEquals},
		{choice.NumericLessThan},
		{choice.NumericGreaterThan},
		{choice.NumericLessThanEquals},
		{choice.NumericGreaterThanEquals},
		{choice.BooleanEquals},
		{choice.TimestampEquals},
		{choice.TimestampLessThan},
		{choice.TimestampGreaterThan},
		{choice.TimestampLessThanEquals},
		{choice.TimestampGreaterThanEquals},
	}

	count := 0
	for _, op := range operators {
		if !isNil(op.field) {
			count++
		}
	}
	return count
}

// Helper function to count compound operators
func countCompoundOperators(choice *ChoiceRule) int {
	count := 0
	if len(choice.And) > 0 {
		count++
	}
	if len(choice.Or) > 0 {
		count++
	}
	if choice.Not != nil {
		count++
	}
	return count
}

// Helper to check if a value is nil (handles interface and pointer types)
func isNil(value interface{}) bool {
	if value == nil {
		return true
	}

	val := reflect.ValueOf(value)
	switch val.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map:
		return val.IsNil()
	default:
		return false
	}
}

// Helper to validate operator requirements
func validateOperatorRequirements(choice *ChoiceRule, index, comparisonCount, compoundCount int) error {
	// For rules with comparison operators, Variable is required
	if comparisonCount > 0 && choice.Variable == "" {
		return fmt.Errorf("choice %d: Variable is required for comparison operators", index)
	}

	// Must have at least one operator
	if comparisonCount == 0 && compoundCount == 0 {
		return fmt.Errorf("choice %d: must have at least one comparison operator or compound operator", index)
	}

	return nil
}

// Helper to validate nested choices recursively
func validateNestedChoices(choice *ChoiceRule, parentIndex int) error {
	// Validate And operators
	for index := range choice.And {
		if err := validateNestedChoice(&choice.And[index], index, parentIndex, "And"); err != nil {
			return err
		}
	}

	// Validate Or operators
	for index := range choice.Or {
		if err := validateNestedChoice(&choice.Or[index], index, parentIndex, "Or"); err != nil {
			return err
		}
	}

	// Validate Not operator
	if choice.Not != nil {
		if err := validateNestedChoice(choice.Not, 0, parentIndex, "Not"); err != nil {
			return err
		}
	}

	return nil
}

// Helper to validate a single nested choice
func validateNestedChoice(choice *ChoiceRule, index, parentIndex int, operatorType string) error {
	// Count operators for nested choice
	comparisonCount := countComparisonOperators(choice)
	compoundCount := countCompoundOperators(choice)

	// Validate operator requirements for nested choice
	if err := validateOperatorRequirements(choice, index, comparisonCount, compoundCount); err != nil {
		return fmt.Errorf("choice %d.%s[%d]: %w", parentIndex, operatorType, index, err)
	}

	// No Next field required for nested choices

	return nil
}

// MarshalJSON implements custom JSON marshaling
func (s *ChoiceState) MarshalJSON() ([]byte, error) {
	// Create a map with the fields we want
	result := map[string]interface{}{
		"Type":    s.Type,
		"Choices": s.Choices,
	}

	// Add optional fields
	if s.Default != nil {
		result["Default"] = s.Default
	}
	if s.InputPath != nil {
		result["InputPath"] = s.InputPath
	}
	if s.ResultPath != nil {
		result["ResultPath"] = s.ResultPath
	}
	if s.OutputPath != nil {
		result["OutputPath"] = s.OutputPath
	}
	if s.Comment != "" {
		result["Comment"] = s.Comment
	}

	return json.Marshal(result)
}

// GetNext returns the next state (nil for Choice states - each choice has its own Next)
func (s *ChoiceState) GetNext() *string {
	return nil
}

// IsEnd returns false for Choice states
func (s *ChoiceState) IsEnd() bool {
	return false
}

// GetNextStates returns all possible next state names from Choice transitions
func (s *ChoiceState) GetNextStates() []string {
	nextStates := make([]string, 0)

	// Add all choice destinations
	for index := range s.Choices {
		nextStates = append(nextStates, s.Choices[index].Next)
	}

	// Add default if present
	if s.Default != nil {
		nextStates = append(nextStates, *s.Default)
	}

	return nextStates
}
