package states

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONPathProcessor_Get(t *testing.T) {
	processor := NewJSONPathProcessor()

	tests := []struct {
		name     string
		data     interface{}
		path     string
		expected interface{}
		hasError bool
	}{
		{
			name:     "root path",
			data:     map[string]interface{}{"key": "value"},
			path:     "$",
			expected: map[string]interface{}{"key": "value"},
		},
		{
			name:     "simple key access",
			data:     map[string]interface{}{"key": "value"},
			path:     "$.key",
			expected: "value",
		},
		{
			name: "nested access",
			data: map[string]interface{}{
				"user": map[string]interface{}{
					"name": "John",
					"age":  30,
				},
			},
			path:     "$.user.name",
			expected: "John",
		},
		{
			name: "array access",
			data: map[string]interface{}{
				"items": []interface{}{"a", "b", "c"},
			},
			path:     "$.items[0]",
			expected: "a",
		},
		{
			name: "nested array access",
			data: map[string]interface{}{
				"users": []interface{}{
					map[string]interface{}{"name": "Alice"},
					map[string]interface{}{"name": "Bob"},
				},
			},
			path:     "$.users[1].name",
			expected: "Bob",
		},
		{
			name:     "non-existent key",
			data:     map[string]interface{}{"key": "value"},
			path:     "$.nonexistent",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.Get(tt.data, tt.path)

			if tt.hasError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestJSONPathProcessor_ApplyResultPath(t *testing.T) {
	processor := NewJSONPathProcessor()

	tests := []struct {
		name     string
		input    interface{}
		result   interface{}
		path     *string
		expected interface{}
	}{
		{
			name:     "no path - return result",
			input:    map[string]interface{}{"a": 1},
			result:   map[string]interface{}{"b": 2},
			path:     nil,
			expected: map[string]interface{}{"b": 2},
		},
		{
			name:     "root path - replace input",
			input:    map[string]interface{}{"a": 1},
			result:   map[string]interface{}{"b": 2},
			path:     StringPtr("$"),
			expected: map[string]interface{}{"b": 2},
		},
		{
			name:   "nested path - merge result",
			input:  map[string]interface{}{"a": 1},
			result: "success",
			path:   StringPtr("$.status"),
			expected: map[string]interface{}{
				"a":      1,
				"status": "success",
			},
		},
		{
			name:   "deep nested path",
			input:  map[string]interface{}{"a": 1},
			result: "data",
			path:   StringPtr("$.level1.level2.value"),
			expected: map[string]interface{}{
				"a": 1,
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"value": "data",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.ApplyResultPath(tt.input, tt.result, tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestJSONPathProcessor_ApplyOutputPath(t *testing.T) {
	processor := NewJSONPathProcessor()

	tests := []struct {
		name     string
		output   interface{}
		path     *string
		expected interface{}
	}{
		{
			name:     "no path - return as-is",
			output:   map[string]interface{}{"key": "value"},
			path:     nil,
			expected: map[string]interface{}{"key": "value"},
		},
		{
			name:     "root path - return as-is",
			output:   map[string]interface{}{"key": "value"},
			path:     StringPtr("$"),
			expected: map[string]interface{}{"key": "value"},
		},
		{
			name: "extract nested value",
			output: map[string]interface{}{
				"data": map[string]interface{}{
					"value": "extracted",
				},
			},
			path:     StringPtr("$.data.value"),
			expected: "extracted",
		},
		{
			name:   "wrap value in new object",
			output: "processed",
			path:   StringPtr("$.result"),
			expected: map[string]interface{}{
				"result": "processed",
			},
		},
		{
			name:   "wrap value with nested path",
			output: "data",
			path:   StringPtr("$.level1.level2.value"),
			expected: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"value": "data",
					},
				},
			},
		},
		{
			name: "extract from array",
			output: map[string]interface{}{
				"items": []interface{}{"first", "second", "third"},
			},
			path:     StringPtr("$.items[1]"),
			expected: "second",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.ApplyOutputPath(tt.output, tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestJSONPathProcessor_ApplyInputPath(t *testing.T) {
	processor := NewJSONPathProcessor()

	tests := []struct {
		name     string
		input    interface{}
		path     *string
		expected interface{}
		hasError bool
	}{
		{
			name:     "nil path returns input",
			input:    map[string]interface{}{"key": "value"},
			path:     nil,
			expected: map[string]interface{}{"key": "value"},
		},
		{
			name:     "empty path returns input",
			input:    map[string]interface{}{"key": "value"},
			path:     StringPtr(""),
			expected: map[string]interface{}{"key": "value"},
		},
		{
			name:     "root path returns input",
			input:    map[string]interface{}{"key": "value"},
			path:     StringPtr("$"),
			expected: map[string]interface{}{"key": "value"},
		},
		{
			name: "extract nested value",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"name": "John",
					"age":  30,
				},
			},
			path:     StringPtr("$.user.name"),
			expected: "John",
		},
		{
			name:     "non-existent path",
			input:    map[string]interface{}{"key": "value"},
			path:     StringPtr("$.nonexistent"),
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.ApplyInputPath(tt.input, tt.path)

			if tt.hasError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestJSONPathProcessor_Set(t *testing.T) {
	processor := NewJSONPathProcessor()

	tests := []struct {
		name     string
		data     interface{}
		path     string
		value    interface{}
		expected interface{}
		hasError bool
	}{
		{
			name:  "set simple field",
			data:  map[string]interface{}{"a": 1},
			path:  "$.b",
			value: 2,
			expected: map[string]interface{}{
				"a": 1,
				"b": 2,
			},
		},
		{
			name:     "replace root",
			data:     map[string]interface{}{"old": "data"},
			path:     "$",
			value:    map[string]interface{}{"new": "data"},
			expected: map[string]interface{}{"new": "data"},
		},
		{
			name:  "create nested structure",
			data:  map[string]interface{}{},
			path:  "$.level1.level2.value",
			value: "data",
			expected: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"value": "data",
					},
				},
			},
		},
		{
			name:     "invalid path",
			data:     map[string]interface{}{},
			path:     "invalid",
			value:    "value",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.Set(tt.data, tt.path, tt.value)

			if tt.hasError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestJSONPathProcessor_ExpandValue(t *testing.T) {
	processor := NewJSONPathProcessor()
	input := map[string]interface{}{
		"user": map[string]interface{}{
			"name": "John",
			"age":  30,
		},
		"items": []interface{}{"a", "b", "c"},
		"order": map[string]interface{}{
			"id":     "ORD-123",
			"amount": 100.50,
		},
	}

	tests := []struct {
		name     string
		value    interface{}
		expected interface{}
	}{
		// ... [existing test cases] ...

		// --- NEW TEST CASES FOR BUG #7 (.$ SYNTAX) ---
		{
			name: "AWS-standard .$ syntax",
			value: map[string]interface{}{
				"orderId.$": "$.order.id",
				"amount.$":  "$.order.amount",
				"name.$":    "$.user.name",
			},
			expected: map[string]interface{}{
				"orderId": "ORD-123",
				"amount":  100.50,
				"name":    "John",
			},
		},
		{
			name: "mixed syntax (.$ and legacy bare JSONPath)",
			value: map[string]interface{}{
				"orderId.$": "$.order.id",    // AWS syntax
				"username":  "$.user.name",   // Legacy syntax
				"static":    "literal value", // Literal
			},
			expected: map[string]interface{}{
				"orderId":  "ORD-123",
				"username": "John",
				"static":   "literal value",
			},
		},
		{
			name: "nested objects with .$ syntax",
			value: map[string]interface{}{
				"data": map[string]interface{}{
					"id.$":   "$.order.id",
					"amount": 99.99, // literal, no .$
				},
			},
			expected: map[string]interface{}{
				"data": map[string]interface{}{
					"id":     "ORD-123",
					"amount": 99.99,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.expandValue(tt.value, input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// NEW TEST: Verify error handling for invalid .$ usage
func TestJSONPathProcessor_ExpandValue_DollarDotErrors(t *testing.T) {
	processor := NewJSONPathProcessor()
	input := map[string]interface{}{"user": "John"}

	// Test: ".$" key with a non-string value should fail
	template := map[string]interface{}{
		"age.$": 30, // Invalid: value must be a string JSONPath
	}

	_, err := processor.expandValue(template, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marked as JSONPath (.$) must have a string value")
}

func TestJSONPathProcessor_MergeMaps(t *testing.T) {
	processor := NewJSONPathProcessor()

	tests := []struct {
		name     string
		mapA     map[string]interface{}
		mapB     map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "simple merge",
			mapA: map[string]interface{}{"a": 1},
			mapB: map[string]interface{}{"b": 2},
			expected: map[string]interface{}{
				"a": 1,
				"b": 2,
			},
		},
		{
			name: "overwrite values",
			mapA: map[string]interface{}{
				"a": 1,
				"b": "old",
			},
			mapB: map[string]interface{}{
				"b": "new",
				"c": 3,
			},
			expected: map[string]interface{}{
				"a": 1,
				"b": "new",
				"c": 3,
			},
		},
		{
			name: "nested merge",
			mapA: map[string]interface{}{
				"level1": map[string]interface{}{
					"a": 1,
				},
			},
			mapB: map[string]interface{}{
				"level1": map[string]interface{}{
					"b": 2,
				},
			},
			expected: map[string]interface{}{
				"level1": map[string]interface{}{
					"a": 1,
					"b": 2,
				},
			},
		},
		{
			name: "nested merge with Nil Object",
			mapA: map[string]interface{}{
				"level1": map[string]interface{}{
					"a": 1,
				},
			},
			mapB: map[string]interface{}{
				"level1": map[string]interface{}{
					"b": 2,
				},
			},
			expected: map[string]interface{}{
				"level1": map[string]interface{}{
					"a": 1,
					"b": 2,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.MergeMaps(tt.mapA, tt.mapB)
			assert.Equal(t, tt.expected, result)
		})
	}
}
