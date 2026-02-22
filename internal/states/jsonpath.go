// Package states provides state machine state implementations and JSONPath processing.
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

		var err error
		// Handle array index
		if p.isArrayIndexPart(part) {
			current, err = p.handleArrayIndex(current, part)
			if err != nil {
				return nil, err
			}
			continue
		}

		// Handle object field
		current, err = p.handleObjectField(current, part)
		if err != nil {
			return nil, err
		}
	}

	return current, nil
}

// isArrayIndexPart checks if a path part is an array index like [0]
func (p *JSONPathProcessor) isArrayIndexPart(part string) bool {
	return part != "" && part[0] == '[' && part[len(part)-1] == ']'
}

// handleArrayIndex handles array indexing for both []interface{} and []map[string]interface{}
func (p *JSONPathProcessor) handleArrayIndex(current interface{}, part string) (interface{}, error) {
	index, err := strconv.Atoi(part[1 : len(part)-1])
	if err != nil {
		return nil, fmt.Errorf("invalid array index: %s", part)
	}

	// Try []interface{} first
	if arr, ok := current.([]interface{}); ok {
		if index < 0 || index >= len(arr) {
			return nil, fmt.Errorf("array index out of bounds")
		}
		return arr[index], nil
	}

	// Try []map[string]interface{}
	if arrOfMaps, ok := current.([]map[string]interface{}); ok {
		if index < 0 || index >= len(arrOfMaps) {
			return nil, fmt.Errorf("array index out of bounds")
		}
		return arrOfMaps[index], nil
	}

	return nil, fmt.Errorf("cannot index non-array")
}

// handleObjectField handles object field access with support for root-level and nested paths
func (p *JSONPathProcessor) handleObjectField(current interface{}, part string) (interface{}, error) {
	rootObj, ok := current.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("cannot access root-field '%s' on type %T", part, current)
	}

	// Check if we have a special '$' key in the root object
	if obj, hasRootKey := rootObj["$"]; hasRootKey {
		// Following the $.key path
		return p.extractFieldValue(obj.(map[string]interface{}), part)
	}

	// Direct key at the root level
	return p.extractFieldValue(rootObj, part)
}

// extractFieldValue extracts a field value from a map
func (p *JSONPathProcessor) extractFieldValue(obj map[string]interface{}, field string) (interface{}, error) {
	val, exists := obj[field]
	if !exists {
		return nil, fmt.Errorf("field '%s' not found", field)
	}
	return val, nil
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
	result := value
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
	result := value
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

// ExpandParameters expands parameter values using JSONPath expressions.
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

// ToJSON converts a value to its JSON string representation.
func (p *JSONPathProcessor) ToJSON(value interface{}) (string, error) {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// FromJSON parses a JSON string into a value.
func (p *JSONPathProcessor) FromJSON(jsonStr string) (interface{}, error) {
	var data interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, err
	}
	return data, nil
}

// SetValue sets a value at the specified JSONPath.
func (p *JSONPathProcessor) SetValue(data interface{}, path string, value interface{}) error {
	_, err := p.setValue(data, path, value)
	return err
}

// Get retrieves a value at the specified JSONPath.
func (p *JSONPathProcessor) Get(data interface{}, path string) (interface{}, error) {
	return p.getValue(data, path)
}

// Set sets a value at the specified JSONPath and returns the modified data.
func (p *JSONPathProcessor) Set(data interface{}, path string, value interface{}) (interface{}, error) {
	return p.setValue(data, path, value)
}

// ExpandValue expands a value using JSONPath expressions from input.
func (p *JSONPathProcessor) ExpandValue(value, input interface{}) (interface{}, error) {
	return p.expandValue(value, input)
}

// MergeMaps merges two maps, with values from b taking precedence over a.
func (p *JSONPathProcessor) MergeMaps(a, b map[string]interface{}) map[string]interface{} {
	return mergeMaps(a, b)
}
