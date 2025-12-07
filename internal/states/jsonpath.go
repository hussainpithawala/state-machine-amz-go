// jsonpath.go - Simplified but correct implementation
package states

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// JSONPathProcessor handles JSONPath operations
type JSONPathProcessor struct{}

// NewJSONPathProcessor creates a new JSONPath processor
func NewJSONPathProcessor() *JSONPathProcessor {
	return &JSONPathProcessor{}
}

// ApplyInputPath applies the input path to the input
func (p *JSONPathProcessor) ApplyInputPath(input interface{}, path *string) (interface{}, error) {
	if path == nil || *path == "" || *path == "$" {
		return input, nil
	}
	return p.getValue(input, *path)
}

// ApplyResultPath applies the result path to combine input and result
func (p *JSONPathProcessor) ApplyResultPath(input, result interface{}, path *string) (interface{}, error) {
	if path == nil || *path == "" {
		return result, nil
	}
	if *path == "$" {
		return result, nil
	}
	return p.setValue(input, *path, result)
}

// ApplyOutputPath applies the output path to filter the output
func (p *JSONPathProcessor) ApplyOutputPath(output interface{}, path *string) (interface{}, error) {
	if path == nil || *path == "" || *path == "$" {
		return output, nil
	}
	value, err := p.getValue(output, *path)
	if err == nil {
		return value, nil
	}
	// If path doesn't exist, wrap the output
	return p.wrapValue(*path, output)
}

// getValue gets a value at a JSONPath
func (p *JSONPathProcessor) getValue(data interface{}, path string) (interface{}, error) {
	if path == "$" {
		return data, nil
	}
	if !strings.HasPrefix(path, "$") {
		return nil, fmt.Errorf("path must start with '$'")
	}

	// Remove $ and optional leading .
	path = strings.TrimPrefix(path, "$")
	path = strings.TrimPrefix(path, ".")

	parts := splitPath(path)
	current := data

	for _, part := range parts {
		if part == "" {
			continue
		}

		// Handle array index
		if part[0] == '[' && part[len(part)-1] == ']' {
			index, err := strconv.Atoi(part[1 : len(part)-1])
			if err != nil {
				return nil, fmt.Errorf("invalid array index: %s", part)
			}

			arr, ok := current.([]interface{})
			if !ok {
				return nil, fmt.Errorf("cannot index non-array")
			}
			if index < 0 || index >= len(arr) {
				return nil, fmt.Errorf("array index out of bounds")
			}
			current = arr[index]
			continue
		}

		// Handle object field
		// Since the root starts with '$.0' we need to first pick the map[string]interface{} from root
		rootObj, ok := current.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("cannot access root-field '%s' on type %T", part, current)
		}
		obj, ok := rootObj["$"]
		if !ok {
			// In this case we directly have the key at the root
			val, exists := rootObj[part]
			if !exists {
				return nil, fmt.Errorf("field '%s' not found", part)
			}
			current = val
		} else {
			// In this case we are following the $.key path
			val, exists := obj.(map[string]interface{})[part]
			if !exists {
				return nil, fmt.Errorf("field '%s' not found", part)
			}
			current = val
		}
	}

	return current, nil
}

// setValue sets a value at a JSONPath
func (p *JSONPathProcessor) setValue(data interface{}, path string, value interface{}) (interface{}, error) {
	if path == "$" {
		return value, nil
	}

	// Validate path starts with "$"
	if !strings.HasPrefix(path, "$") {
		return nil, fmt.Errorf("path must start with '$'")
	}

	// Parse the path
	parts := splitPath(strings.TrimPrefix(strings.TrimPrefix(path, "$"), "."))

	// Start with the value and wrap it
	var result interface{} = value
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]
		if part == "" {
			continue
		}

		if part[0] == '[' && part[len(part)-1] == ']' {
			// Array index - create array
			index, err := strconv.Atoi(part[1 : len(part)-1])
			if err != nil {
				return nil, fmt.Errorf("invalid array index: %s", part)
			}

			// Create array with value at index
			arr := make([]interface{}, index+1)
			arr[index] = result
			result = arr
		} else {
			// Object field
			result = map[string]interface{}{part: result}
		}
	}

	// Merge with original data if it's a map
	if dataMap, ok := data.(map[string]interface{}); ok {
		return mergeMaps(dataMap, result.(map[string]interface{})), nil
	}

	return result, nil
}

// wrapValue wraps a value in a nested structure based on path
func (p *JSONPathProcessor) wrapValue(path string, value interface{}) (interface{}, error) {
	if path == "$" {
		return value, nil
	}

	// Validate path starts with "$"
	if !strings.HasPrefix(path, "$") {
		return nil, fmt.Errorf("path must start with '$'")
	}

	// Parse the path
	parts := splitPath(strings.TrimPrefix(strings.TrimPrefix(path, "$"), "."))

	// Start with the value and wrap it
	var result interface{} = value
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]
		if part == "" {
			continue
		}

		if part[0] == '[' && part[len(part)-1] == ']' {
			// Array index - create array
			index, err := strconv.Atoi(part[1 : len(part)-1])
			if err != nil {
				return nil, fmt.Errorf("invalid array index: %s", part)
			}

			// Create array with value at index
			arr := make([]interface{}, index+1)
			arr[index] = result
			result = arr
		} else {
			// Object field
			result = map[string]interface{}{part: result}
		}
	}

	return result, nil
}

// splitPath splits a JSONPath into parts
func splitPath(path string) []string {
	var parts []string
	var current strings.Builder
	inBrackets := false

	for i := 0; i < len(path); i++ {
		ch := path[i]

		switch ch {
		case '.':
			if !inBrackets {
				if current.Len() > 0 {
					parts = append(parts, current.String())
					current.Reset()
				}
			} else {
				current.WriteByte(ch)
			}
		case '[':
			if !inBrackets {
				if current.Len() > 0 {
					parts = append(parts, current.String())
					current.Reset()
				}
				inBrackets = true
				current.WriteByte(ch)
			} else {
				// Nested brackets - just add to current
				current.WriteByte(ch)
			}
		case ']':
			if inBrackets {
				current.WriteByte(ch)
				parts = append(parts, current.String())
				current.Reset()
				inBrackets = false
			} else {
				// Unmatched closing bracket
				current.WriteByte(ch)
			}
		default:
			current.WriteByte(ch)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// mergeMaps deeply merges two maps
func mergeMaps(a, b map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy all from a
	for k, v := range a {
		result[k] = v
	}

	// Merge from b
	for k, v := range b {
		if existing, exists := result[k]; exists {
			// Both have this key
			if aMap, ok := existing.(map[string]interface{}); ok {
				if bMap, ok := v.(map[string]interface{}); ok {
					// Both are maps, merge recursively
					result[k] = mergeMaps(aMap, bMap)
					continue
				}
			}
			// Not both maps or different types, b wins
		}
		result[k] = v
	}

	return result
}

// Other helper methods...
func (p *JSONPathProcessor) ExpandParameters(params map[string]interface{}, input interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	for k, v := range params {
		expanded, err := p.expandValue(v, input)
		if err != nil {
			return nil, fmt.Errorf("failed to expand parameter '%s': %w", k, err)
		}
		result[k] = expanded
	}
	return result, nil
}

func (p *JSONPathProcessor) expandValue(value, input interface{}) (interface{}, error) {
	switch v := value.(type) {
	case string:
		if strings.HasPrefix(v, "$") {
			return p.getValue(input, v)
		}
		return v, nil
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, val := range v {
			expanded, err := p.expandValue(val, input)
			if err != nil {
				return nil, err
			}
			result[key] = expanded
		}
		return result, nil
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, val := range v {
			expanded, err := p.expandValue(val, input)
			if err != nil {
				return nil, err
			}
			result[i] = expanded
		}
		return result, nil
	default:
		return value, nil
	}
}

func (p *JSONPathProcessor) ToJSON(value interface{}) (string, error) {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (p *JSONPathProcessor) FromJSON(jsonStr string) (interface{}, error) {
	var data interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, err
	}
	return data, nil
}

func (p *JSONPathProcessor) SetValue(data interface{}, path string, value interface{}) error {
	_, err := p.setValue(data, path, value)
	return err
}

// Export methods for testing
func (p *JSONPathProcessor) Get(data interface{}, path string) (interface{}, error) {
	return p.getValue(data, path)
}

func (p *JSONPathProcessor) Set(data interface{}, path string, value interface{}) (interface{}, error) {
	return p.setValue(data, path, value)
}

func (p *JSONPathProcessor) ExpandValue(value, input interface{}) (interface{}, error) {
	return p.expandValue(value, input)
}

func (p *JSONPathProcessor) MergeMaps(a, b map[string]interface{}) map[string]interface{} {
	return mergeMaps(a, b)
}
