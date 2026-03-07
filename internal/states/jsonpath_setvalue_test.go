package states

import (
	"encoding/json"
	"testing"
)

func TestJSONPathProcessor_SetValue_WithArrayPaths(t *testing.T) {
	processor := NewJSONPathProcessor()

	tests := []struct {
		name     string
		data     interface{}
		path     string
		value    interface{}
		expected interface{}
		wantErr  bool
	}{
		{
			name:  "Set value in existing map",
			data:  map[string]interface{}{"existing": "value"},
			path:  "$.newField",
			value: "newValue",
			expected: map[string]interface{}{
				"existing": "value",
				"newField": "newValue",
			},
			wantErr: false,
		},
		{
			name:  "Set value with nested path",
			data:  map[string]interface{}{"existing": "value"},
			path:  "$.nested.field",
			value: "nestedValue",
			expected: map[string]interface{}{
				"existing": "value",
				"nested": map[string]interface{}{
					"field": "nestedValue",
				},
			},
			wantErr: false,
		},
		{
			name:  "Set value with array path - creates array",
			data:  map[string]interface{}{"existing": "value"},
			path:  "$.items[0]",
			value: "firstItem",
			expected: map[string]interface{}{
				"existing": "value",
				"items":    []interface{}{"firstItem"},
			},
			wantErr: false,
		},
		{
			name:  "Set value at root",
			data:  map[string]interface{}{"old": "data"},
			path:  "$",
			value: map[string]interface{}{"new": "data"},
			expected: map[string]interface{}{
				"new": "data",
			},
			wantErr: false,
		},
		{
			name:  "Set value on empty data",
			data:  map[string]interface{}{},
			path:  "$.field",
			value: "value",
			expected: map[string]interface{}{
				"field": "value",
			},
			wantErr: false,
		},
		{
			name:  "Set complex nested structure",
			data:  map[string]interface{}{"root": "value"},
			path:  "$.a.b.c",
			value: "deep",
			expected: map[string]interface{}{
				"root": "value",
				"a": map[string]interface{}{
					"b": map[string]interface{}{
						"c": "deep",
					},
				},
			},
			wantErr: false,
		},
		{
			name:  "Set value creates array in path",
			data:  map[string]interface{}{},
			path:  "$.list[2]",
			value: "third",
			expected: map[string]interface{}{
				"list": []interface{}{nil, nil, "third"},
			},
			wantErr: false,
		},
		{
			name:  "Overwrite existing field",
			data:  map[string]interface{}{"field": "old"},
			path:  "$.field",
			value: "new",
			expected: map[string]interface{}{
				"field": "new",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.setValue(tt.data, tt.path, tt.value)

			if (err != nil) != tt.wantErr {
				t.Errorf("setValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// Compare as JSON to handle nested structures properly
			expectedJSON, _ := json.Marshal(tt.expected)
			resultJSON, _ := json.Marshal(result)

			if string(expectedJSON) != string(resultJSON) {
				t.Errorf("setValue() result mismatch\nExpected: %s\nGot:      %s",
					string(expectedJSON), string(resultJSON))
			}
		})
	}
}

func TestJSONPathProcessor_SetValue_NonMapData(t *testing.T) {
	processor := NewJSONPathProcessor()

	tests := []struct {
		name     string
		data     interface{}
		path     string
		value    interface{}
		expected interface{}
	}{
		{
			name:  "Set on nil data creates new structure",
			data:  nil,
			path:  "$.field",
			value: "value",
			expected: map[string]interface{}{
				"field": "value",
			},
		},
		{
			name:  "Set on string data creates new structure",
			data:  "string data",
			path:  "$.field",
			value: "value",
			expected: map[string]interface{}{
				"field": "value",
			},
		},
		{
			name:  "Set on array data creates new structure",
			data:  []interface{}{1, 2, 3},
			path:  "$.field",
			value: "value",
			expected: map[string]interface{}{
				"field": "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.setValue(tt.data, tt.path, tt.value)
			if err != nil {
				t.Errorf("setValue() unexpected error = %v", err)
				return
			}

			expectedJSON, _ := json.Marshal(tt.expected)
			resultJSON, _ := json.Marshal(result)

			if string(expectedJSON) != string(resultJSON) {
				t.Errorf("setValue() result mismatch\nExpected: %s\nGot:      %s",
					string(expectedJSON), string(resultJSON))
			}
		})
	}
}

func TestJSONPathProcessor_SetValue_ErrorCases(t *testing.T) {
	processor := NewJSONPathProcessor()

	tests := []struct {
		name    string
		data    interface{}
		path    string
		value   interface{}
		wantErr bool
	}{
		{
			name:    "Invalid path - no $",
			data:    map[string]interface{}{},
			path:    "field",
			value:   "value",
			wantErr: true,
		},
		{
			name:    "Invalid array index",
			data:    map[string]interface{}{},
			path:    "$.field[abc]",
			value:   "value",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := processor.setValue(tt.data, tt.path, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("setValue() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
