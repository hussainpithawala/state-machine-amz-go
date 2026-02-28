package states

import (
	"encoding/json"
	"testing"
)

func TestWaitState_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		state    *WaitState
		expected map[string]interface{}
	}{
		{
			name: "Wait with Seconds",
			state: &WaitState{
				BaseState: BaseState{
					Name: "WaitState",
					Next: stringPtr("NextState"),
				},
				Seconds: int64Ptr(10),
			},
			expected: map[string]interface{}{
				"Type":    "Wait",
				"Next":    "NextState",
				"Seconds": float64(10), // JSON unmarshals numbers as float64
			},
		},
		{
			name: "Wait with SecondsPath",
			state: &WaitState{
				BaseState: BaseState{
					Name: "WaitState",
					Next: stringPtr("NextState"),
				},
				SecondsPath: stringPtr("$.waitTime"),
			},
			expected: map[string]interface{}{
				"Type":        "Wait",
				"Next":        "NextState",
				"SecondsPath": "$.waitTime",
			},
		},
		{
			name: "Wait with Timestamp",
			state: &WaitState{
				BaseState: BaseState{
					Name: "WaitState",
					End:  true,
				},
				Timestamp: stringPtr("2024-12-31T23:59:59Z"),
			},
			expected: map[string]interface{}{
				"Type":      "Wait",
				"End":       true,
				"Timestamp": "2024-12-31T23:59:59Z",
			},
		},
		{
			name: "Wait with TimestampPath",
			state: &WaitState{
				BaseState: BaseState{
					Name:       "WaitState",
					Next:       stringPtr("NextState"),
					InputPath:  stringPtr("$.input"),
					OutputPath: stringPtr("$.output"),
					ResultPath: stringPtr("$.result"),
					Comment:    "Wait until timestamp",
				},
				TimestampPath: stringPtr("$.scheduledTime"),
			},
			expected: map[string]interface{}{
				"Type":          "Wait",
				"Next":          "NextState",
				"InputPath":     "$.input",
				"OutputPath":    "$.output",
				"ResultPath":    "$.result",
				"Comment":       "Wait until timestamp",
				"TimestampPath": "$.scheduledTime",
			},
		},
		{
			name: "Wait with all base state fields",
			state: &WaitState{
				BaseState: BaseState{
					Name:       "ComplexWait",
					Next:       stringPtr("NextState"),
					InputPath:  stringPtr("$.data"),
					OutputPath: stringPtr("$.output"),
					ResultPath: stringPtr("$.waitResult"),
					Comment:    "A complex wait state",
				},
				Seconds: int64Ptr(5),
			},
			expected: map[string]interface{}{
				"Type":       "Wait",
				"Next":       "NextState",
				"InputPath":  "$.data",
				"OutputPath": "$.output",
				"ResultPath": "$.waitResult",
				"Comment":    "A complex wait state",
				"Seconds":    float64(5),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal the state
			jsonBytes, err := json.Marshal(tt.state)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}

			// Unmarshal to map to compare
			var result map[string]interface{}
			if err := json.Unmarshal(jsonBytes, &result); err != nil {
				t.Fatalf("Failed to unmarshal result: %v", err)
			}

			// Check each expected field
			for key, expectedValue := range tt.expected {
				actualValue, exists := result[key]
				if !exists {
					t.Errorf("Expected field '%s' is missing from marshaled JSON", key)
					continue
				}

				// Compare values
				if !compareValues(expectedValue, actualValue) {
					t.Errorf("Field '%s': expected %v (%T), got %v (%T)",
						key, expectedValue, expectedValue, actualValue, actualValue)
				}
			}

			// Check that no unexpected fields are present (except fields that might be zero-valued)
			for key := range result {
				if _, expected := tt.expected[key]; !expected {
					t.Errorf("Unexpected field '%s' in marshaled JSON with value: %v", key, result[key])
				}
			}
		})
	}
}

func TestWaitState_MarshalJSON_AllWaitTypes(t *testing.T) {
	// Test that all four wait type fields are properly marshaled
	seconds := int64(10)
	secondsPath := "$.seconds"
	timestamp := "2024-12-31T23:59:59Z"
	timestampPath := "$.time"

	// Test each individually
	testCases := []struct {
		name  string
		state *WaitState
		field string
	}{
		{
			name: "Seconds field",
			state: &WaitState{
				BaseState: BaseState{Name: "W1"},
				Seconds:   &seconds,
			},
			field: "Seconds",
		},
		{
			name: "SecondsPath field",
			state: &WaitState{
				BaseState:   BaseState{Name: "W2"},
				SecondsPath: &secondsPath,
			},
			field: "SecondsPath",
		},
		{
			name: "Timestamp field",
			state: &WaitState{
				BaseState: BaseState{Name: "W3"},
				Timestamp: &timestamp,
			},
			field: "Timestamp",
		},
		{
			name: "TimestampPath field",
			state: &WaitState{
				BaseState:     BaseState{Name: "W4"},
				TimestampPath: &timestampPath,
			},
			field: "TimestampPath",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jsonBytes, err := json.Marshal(tc.state)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}

			var result map[string]interface{}
			if err := json.Unmarshal(jsonBytes, &result); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if _, exists := result[tc.field]; !exists {
				t.Errorf("Field '%s' is missing from marshaled JSON. Got: %s",
					tc.field, string(jsonBytes))
			}
		})
	}
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func int64Ptr(i int64) *int64 {
	return &i
}

func compareValues(expected, actual interface{}) bool {
	// Handle type conversions for numbers (JSON unmarshals all numbers as float64)
	switch e := expected.(type) {
	case int:
		if a, ok := actual.(float64); ok {
			return float64(e) == a
		}
	case int64:
		if a, ok := actual.(float64); ok {
			return float64(e) == a
		}
	case float64:
		if a, ok := actual.(float64); ok {
			return e == a
		}
	}

	// Direct comparison for other types
	return expected == actual
}
