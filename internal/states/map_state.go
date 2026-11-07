package states

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
)

// ItemProcessor defines the sub-state-machine that processes each item in a Map state
type ItemProcessor struct {
	StartAt         string                 `json:"StartAt"`
	States          map[string]State       `json:"States"`
	ProcessorConfig map[string]interface{} `json:"ProcessorConfig,omitempty"`
}

// ItemBatcherConfig defines how to batch items before processing
type ItemBatcherConfig struct {
	BatchInput                map[string]interface{} `json:"BatchInput,omitempty"`
	MaxItemsPerBatch          *int                   `json:"MaxItemsPerBatch,omitempty"`
	MaxItemsPerBatchPath      *string                `json:"MaxItemsPerBatchPath,omitempty"`
	MaxInputBytesPerBatch     *int                   `json:"MaxInputBytesPerBatch,omitempty"`
	MaxInputBytesPerBatchPath *string                `json:"MaxInputBytesPerBatchPath,omitempty"`
}

// MapState represents an AWS Map state
type MapState struct {
	BaseState
	ItemsPath                      *string                `json:"ItemsPath,omitempty"`
	ItemSelector                   map[string]interface{} `json:"ItemSelector,omitempty"`
	Parameters                     map[string]interface{} `json:"Parameters,omitempty"`
	ItemProcessor_                 json.RawMessage        `json:"ItemProcessor,omitempty"`
	Iterator_                      json.RawMessage        `json:"Iterator,omitempty"`
	MaxConcurrency                 *int                   `json:"MaxConcurrency,omitempty"`
	MaxConcurrencyPath             *string                `json:"MaxConcurrencyPath,omitempty"`
	ToleratedFailurePercentage     *float64               `json:"ToleratedFailurePercentage,omitempty"`
	ToleratedFailurePercentagePath *string                `json:"ToleratedFailurePercentagePath,omitempty"`
	ToleratedFailureCount          *int                   `json:"ToleratedFailureCount,omitempty"`
	ToleratedFailureCountPath      *string                `json:"ToleratedFailureCountPath,omitempty"`
	ItemBatcher                    *ItemBatcherConfig     `json:"ItemBatcher,omitempty"`
	ResultSelector                 map[string]interface{} `json:"ResultSelector,omitempty"`
	Retry                          []RetryPolicy          `json:"Retry,omitempty"`
	Catch                          []CatchPolicy          `json:"Catch,omitempty"`

	// Parsed at unmarshal time
	itemProcessor *ItemProcessor
}

func (m *MapState) GetName() string  { return m.Name }
func (m *MapState) GetType() string  { return "Map" }
func (m *MapState) GetNext() *string { return m.Next }
func (m *MapState) IsEnd() bool      { return m.End }

func (m *MapState) GetNextStates() []string {
	nextStates := make([]string, 0)
	if m.Next != nil {
		nextStates = append(nextStates, *m.Next)
	}
	for _, catchPolicy := range m.Catch {
		nextStates = append(nextStates, catchPolicy.Next)
	}
	return nextStates
}

// UnmarshalJSON implements custom JSON unmarshaling for MapState
func (m *MapState) UnmarshalJSON(data []byte) error {
	type Alias MapState
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(m),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Parse ItemProcessor (or deprecated Iterator)
	rawProcessor := m.ItemProcessor_
	if rawProcessor == nil {
		rawProcessor = m.Iterator_
	}
	if rawProcessor != nil {
		if err := m.parseItemProcessor(rawProcessor); err != nil {
			return fmt.Errorf("failed to parse ItemProcessor: %w", err)
		}
	}

	return nil
}

func (m *MapState) parseItemProcessor(raw json.RawMessage) error {
	var processorData struct {
		StartAt         string                     `json:"StartAt"`
		States          map[string]json.RawMessage `json:"States"`
		ProcessorConfig map[string]interface{}     `json:"ProcessorConfig,omitempty"`
	}

	if err := json.Unmarshal(raw, &processorData); err != nil {
		return err
	}

	states := make(map[string]State)
	unmarshaler := &stateUnmarshaler{}
	for stateName, rawState := range processorData.States {
		state, err := unmarshaler.unmarshalState(stateName, rawState)
		if err != nil {
			return fmt.Errorf("failed to unmarshal state '%s' in ItemProcessor: %w", stateName, err)
		}
		states[stateName] = state
	}

	m.itemProcessor = &ItemProcessor{
		StartAt:         processorData.StartAt,
		States:          states,
		ProcessorConfig: processorData.ProcessorConfig,
	}

	return nil
}

// Validate validates the Map state configuration
func (m *MapState) Validate() error {
	if err := m.validateItemProcessor(); err != nil {
		return err
	}
	if err := m.validateConcurrencyFields(); err != nil {
		return err
	}
	if err := m.validateFailureToleranceFields(); err != nil {
		return err
	}
	if m.ItemSelector != nil && m.Parameters != nil {
		return fmt.Errorf("map state '%s' must not have both ItemSelector and Parameters", m.Name)
	}
	if m.ItemBatcher != nil {
		return m.validateItemBatcher()
	}
	return nil
}

func (m *MapState) validateItemProcessor() error {
	if m.itemProcessor == nil {
		return fmt.Errorf("map state '%s' must have an ItemProcessor or Iterator", m.Name)
	}
	if m.itemProcessor.StartAt == "" {
		return fmt.Errorf("map state '%s' ItemProcessor: StartAt is required", m.Name)
	}
	if len(m.itemProcessor.States) == 0 {
		return fmt.Errorf("map state '%s' ItemProcessor: States must not be empty", m.Name)
	}
	if _, exists := m.itemProcessor.States[m.itemProcessor.StartAt]; !exists {
		return fmt.Errorf("map state '%s' ItemProcessor: StartAt state '%s' not found", m.Name, m.itemProcessor.StartAt)
	}
	for stateName, state := range m.itemProcessor.States {
		if !state.IsEnd() && state.GetNext() == nil {
			return fmt.Errorf("map state '%s' ItemProcessor: state '%s' must have either Next or End", m.Name, stateName)
		}
	}
	return nil
}

func (m *MapState) validateConcurrencyFields() error {
	if m.MaxConcurrency != nil && m.MaxConcurrencyPath != nil {
		return fmt.Errorf("map state '%s' must not have both MaxConcurrency and MaxConcurrencyPath", m.Name)
	}
	if m.MaxConcurrency != nil && *m.MaxConcurrency < 0 {
		return fmt.Errorf("map state '%s' MaxConcurrency must be non-negative", m.Name)
	}
	return nil
}

func (m *MapState) validateFailureToleranceFields() error {
	if m.ToleratedFailurePercentage != nil && m.ToleratedFailurePercentagePath != nil {
		return fmt.Errorf("map state '%s' must not have both ToleratedFailurePercentage and ToleratedFailurePercentagePath", m.Name)
	}
	if m.ToleratedFailurePercentage != nil && (*m.ToleratedFailurePercentage < 0 || *m.ToleratedFailurePercentage > 100) {
		return fmt.Errorf("map state '%s' ToleratedFailurePercentage must be between 0 and 100", m.Name)
	}
	if m.ToleratedFailureCount != nil && m.ToleratedFailureCountPath != nil {
		return fmt.Errorf("map state '%s' must not have both ToleratedFailureCount and ToleratedFailureCountPath", m.Name)
	}
	if m.ToleratedFailureCount != nil && *m.ToleratedFailureCount < 0 {
		return fmt.Errorf("map state '%s' ToleratedFailureCount must be non-negative", m.Name)
	}
	return nil
}

func (m *MapState) validateItemBatcher() error {
	b := m.ItemBatcher
	if b.MaxItemsPerBatch != nil && b.MaxItemsPerBatchPath != nil {
		return fmt.Errorf("map state '%s' ItemBatcher must not have both MaxItemsPerBatch and MaxItemsPerBatchPath", m.Name)
	}
	if b.MaxInputBytesPerBatch != nil && b.MaxInputBytesPerBatchPath != nil {
		return fmt.Errorf("map state '%s' ItemBatcher must not have both MaxInputBytesPerBatch and MaxInputBytesPerBatchPath", m.Name)
	}
	hasAny := b.MaxItemsPerBatch != nil || b.MaxItemsPerBatchPath != nil ||
		b.MaxInputBytesPerBatch != nil || b.MaxInputBytesPerBatchPath != nil
	if !hasAny {
		return fmt.Errorf("map state '%s' ItemBatcher must contain at least one of MaxItemsPerBatch, MaxItemsPerBatchPath, MaxInputBytesPerBatch, or MaxInputBytesPerBatchPath", m.Name)
	}
	return nil
}

// Execute executes the Map state
func (m *MapState) Execute(ctx context.Context, input interface{}) (result interface{}, next *string, err error) {
	processor := NewJSONPathProcessor()

	processedInput, err := processor.ApplyInputPath(input, m.InputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply input path: %w", err)
	}

	// Resolve the items array
	itemsArray, err := m.resolveItemsArray(processor, processedInput)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve items array: %w", err)
	}

	// Resolve MaxConcurrency
	maxConcurrency, err := m.resolveMaxConcurrency(processor, processedInput)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve MaxConcurrency: %w", err)
	}

	// Resolve failure tolerance
	toleratedFailureCount, toleratedFailurePercentage, err := m.resolveFailureTolerance(processor, processedInput)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve failure tolerance: %w", err)
	}

	// Build selected items (apply ItemSelector/Parameters to each element)
	selectedItems, err := m.buildSelectedItems(processor, processedInput, itemsArray)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build selected items: %w", err)
	}

	// Batch items if ItemBatcher is configured
	batches, err := m.batchItems(processor, processedInput, selectedItems)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to batch items: %w", err)
	}

	// Execute iterations
	results, err := m.executeIterations(ctx, batches, maxConcurrency, toleratedFailureCount, toleratedFailurePercentage)
	if err != nil {
		return nil, nil, err
	}

	// Apply ResultSelector
	var output interface{} = results
	if m.ResultSelector != nil {
		output, err = processor.expandValue(m.ResultSelector, map[string]interface{}{
			"$": results,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to apply ResultSelector: %w", err)
		}
	}

	// Apply ResultPath
	output, err = processor.ApplyResultPath(processedInput, output, m.ResultPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply result path: %w", err)
	}

	// Apply OutputPath
	output, err = processor.ApplyOutputPath(output, m.OutputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply output path: %w", err)
	}

	return output, m.GetNext(), nil
}

func (m *MapState) resolveItemsArray(processor *JSONPathProcessor, input interface{}) ([]interface{}, error) {
	itemsPath := m.ItemsPath
	if itemsPath == nil {
		defaultPath := "$"
		itemsPath = &defaultPath
	}

	rawItems, err := processor.ApplyInputPath(input, itemsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve ItemsPath '%s': %w", *itemsPath, err)
	}

	arr, ok := rawItems.([]interface{})
	if !ok {
		return nil, fmt.Errorf("ItemsPath must resolve to a JSON array, got %T", rawItems)
	}

	return arr, nil
}

func (m *MapState) resolveMaxConcurrency(processor *JSONPathProcessor, input interface{}) (int, error) {
	if m.MaxConcurrency != nil {
		return *m.MaxConcurrency, nil
	}
	if m.MaxConcurrencyPath != nil {
		val, err := processor.getValue(input, *m.MaxConcurrencyPath)
		if err != nil {
			return 0, fmt.Errorf("failed to resolve MaxConcurrencyPath: %w", err)
		}
		return toInt(val)
	}
	return 0, nil
}

func (m *MapState) resolveFailureTolerance(processor *JSONPathProcessor, input interface{}) (count int, percentage float64, err error) {
	failureCount := 0
	failurePercentage := 0.0

	if m.ToleratedFailureCount != nil {
		failureCount = *m.ToleratedFailureCount
	} else if m.ToleratedFailureCountPath != nil {
		val, err := processor.getValue(input, *m.ToleratedFailureCountPath)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to resolve ToleratedFailureCountPath: %w", err)
		}
		failureCount, err = toInt(val)
		if err != nil {
			return 0, 0, err
		}
	}

	if m.ToleratedFailurePercentage != nil {
		failurePercentage = *m.ToleratedFailurePercentage
	} else if m.ToleratedFailurePercentagePath != nil {
		val, err := processor.getValue(input, *m.ToleratedFailurePercentagePath)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to resolve ToleratedFailurePercentagePath: %w", err)
		}
		f, err := toFloat(val)
		if err != nil {
			return 0, 0, err
		}
		failurePercentage = f
	}

	return failureCount, failurePercentage, nil
}

func (m *MapState) buildSelectedItems(processor *JSONPathProcessor, fullInput interface{}, items []interface{}) ([]interface{}, error) {
	selector := m.ItemSelector
	if selector == nil {
		selector = m.Parameters
	}

	if selector == nil {
		return items, nil
	}

	selected := make([]interface{}, len(items))
	for i, item := range items {
		contextObj := m.buildMapContext(i, item)
		resolved, err := m.expandItemSelector(processor, selector, fullInput, contextObj)
		if err != nil {
			return nil, fmt.Errorf("failed to expand ItemSelector for item %d: %w", i, err)
		}
		selected[i] = resolved
	}

	return selected, nil
}

// expandItemSelector expands an ItemSelector template, resolving both "$" (input)
// and "$$" (context object) references in "key.$" values.
func (m *MapState) expandItemSelector(processor *JSONPathProcessor, template, input interface{}, contextObj map[string]interface{}) (interface{}, error) {
	switch v := template.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, val := range v {
			if strings.HasSuffix(key, ".$") {
				outputKey := strings.TrimSuffix(key, ".$")
				strVal, ok := val.(string)
				if !ok {
					return nil, fmt.Errorf("key '%s' marked as JSONPath (.$) must have a string value", key)
				}
				evaluated, err := m.resolveItemSelectorPath(processor, strVal, input, contextObj)
				if err != nil {
					return nil, fmt.Errorf("failed to evaluate '%s' for key '%s': %w", strVal, outputKey, err)
				}
				result[outputKey] = evaluated
				continue
			}
			expanded, err := m.expandItemSelector(processor, val, input, contextObj)
			if err != nil {
				return nil, err
			}
			result[key] = expanded
		}
		return result, nil

	case string:
		if strings.HasPrefix(v, "$$") {
			return processor.getValue(contextObj, strings.Replace(v, "$$", "$", 1))
		}
		if strings.HasPrefix(v, "$") {
			return processor.getValue(input, v)
		}
		return v, nil

	case []interface{}:
		result := make([]interface{}, len(v))
		for i, val := range v {
			expanded, err := m.expandItemSelector(processor, val, input, contextObj)
			if err != nil {
				return nil, err
			}
			result[i] = expanded
		}
		return result, nil

	default:
		return template, nil
	}
}

func (m *MapState) resolveItemSelectorPath(processor *JSONPathProcessor, path string, input interface{}, contextObj map[string]interface{}) (interface{}, error) {
	if strings.HasPrefix(path, "$$.") || path == "$$" {
		ctxPath := strings.Replace(path, "$$", "$", 1)
		return processor.getValue(contextObj, ctxPath)
	}
	return processor.getValue(input, path)
}

func (m *MapState) buildMapContext(index int, value interface{}) map[string]interface{} {
	return map[string]interface{}{
		"Map": map[string]interface{}{
			"Item": map[string]interface{}{
				"Index": index,
				"Value": value,
			},
		},
	}
}

func (m *MapState) batchItems(processor *JSONPathProcessor, input interface{}, items []interface{}) ([]interface{}, error) {
	if m.ItemBatcher == nil {
		return items, nil
	}

	maxPerBatch, err := m.resolveMaxItemsPerBatch(processor, input)
	if err != nil {
		return nil, err
	}

	maxBytesPerBatch, err := m.resolveMaxBytesPerBatch(processor, input)
	if err != nil {
		return nil, err
	}

	var batches []interface{}
	currentBatch := make([]interface{}, 0, maxPerBatch)
	currentBytes := 0

	flushBatch := func() {
		if len(currentBatch) == 0 {
			return
		}
		var batchItem interface{} = currentBatch
		if m.ItemBatcher.BatchInput != nil {
			batchItem = map[string]interface{}{
				"BatchInput": m.ItemBatcher.BatchInput,
				"Items":      currentBatch,
			}
		}
		batches = append(batches, batchItem)
		currentBatch = nil
		currentBytes = 0
	}

	for _, item := range items {
		itemBytes := 0
		if maxBytesPerBatch > 0 {
			b, err := json.Marshal(item)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal item for size calculation: %w", err)
			}
			itemBytes = len(b)
		}

		if maxBytesPerBatch > 0 && currentBytes+itemBytes > maxBytesPerBatch && len(currentBatch) > 0 {
			flushBatch()
		}

		currentBatch = append(currentBatch, item)
		currentBytes += itemBytes

		if maxPerBatch > 0 && len(currentBatch) >= maxPerBatch {
			flushBatch()
		}
	}

	flushBatch()

	return batches, nil
}

func (m *MapState) resolveMaxItemsPerBatch(processor *JSONPathProcessor, input interface{}) (int, error) {
	if m.ItemBatcher.MaxItemsPerBatch != nil {
		return *m.ItemBatcher.MaxItemsPerBatch, nil
	}
	if m.ItemBatcher.MaxItemsPerBatchPath != nil {
		val, err := processor.getValue(input, *m.ItemBatcher.MaxItemsPerBatchPath)
		if err != nil {
			return 0, err
		}
		return toInt(val)
	}
	return 0, nil
}

func (m *MapState) resolveMaxBytesPerBatch(processor *JSONPathProcessor, input interface{}) (int, error) {
	if m.ItemBatcher.MaxInputBytesPerBatch != nil {
		return *m.ItemBatcher.MaxInputBytesPerBatch, nil
	}
	if m.ItemBatcher.MaxInputBytesPerBatchPath != nil {
		val, err := processor.getValue(input, *m.ItemBatcher.MaxInputBytesPerBatchPath)
		if err != nil {
			return 0, err
		}
		return toInt(val)
	}
	return 0, nil
}

type iterationConfig struct {
	items                      []interface{}
	maxConcurrency             int
	toleratedFailureCount      int
	toleratedFailurePercentage float64
}

func (m *MapState) executeIterations(ctx context.Context, items []interface{}, maxConcurrency, toleratedFailureCount int, toleratedFailurePercentage float64) ([]interface{}, error) {
	if len(items) == 0 {
		return []interface{}{}, nil
	}

	cfg := &iterationConfig{
		items:                      items,
		maxConcurrency:             maxConcurrency,
		toleratedFailureCount:      toleratedFailureCount,
		toleratedFailurePercentage: toleratedFailurePercentage,
	}

	if maxConcurrency == 1 {
		return m.executeSequential(ctx, cfg)
	}
	return m.executeConcurrent(ctx, cfg)
}

func (m *MapState) executeSequential(ctx context.Context, cfg *iterationConfig) ([]interface{}, error) {
	total := len(cfg.items)
	results := make([]interface{}, total)
	var failures int

	for i, item := range cfg.items {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		result, err := m.executeIteration(ctx, item)
		results[i] = result

		if err != nil {
			failures++
			if m.exceedsFailureThreshold(failures, total, cfg.toleratedFailureCount, cfg.toleratedFailurePercentage) {
				return nil, m.failureThresholdError(failures, total)
			}
		}
	}
	return results, nil
}

func (m *MapState) executeConcurrent(ctx context.Context, cfg *iterationConfig) ([]interface{}, error) {
	total := len(cfg.items)
	results := make([]interface{}, total)
	sem := make(chan struct{}, m.effectiveConcurrency(cfg.maxConcurrency, total))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failureCount int64
	var abortErr error

	for i, item := range cfg.items {
		mu.Lock()
		if abortErr != nil {
			mu.Unlock()
			break
		}
		mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(index int, iterItem interface{}) {
			defer wg.Done()
			defer func() { <-sem }()

			result, err := m.executeIteration(ctx, iterItem)

			mu.Lock()
			defer mu.Unlock()

			results[index] = result
			if err != nil {
				fc := int(atomic.AddInt64(&failureCount, 1))
				if m.exceedsFailureThreshold(fc, total, cfg.toleratedFailureCount, cfg.toleratedFailurePercentage) {
					abortErr = m.failureThresholdError(fc, total)
				}
			}
		}(i, item)
	}

	wg.Wait()

	if abortErr != nil {
		return nil, abortErr
	}

	fc := int(atomic.LoadInt64(&failureCount))
	if fc > 0 && m.exceedsFailureThreshold(fc, total, cfg.toleratedFailureCount, cfg.toleratedFailurePercentage) {
		return nil, m.failureThresholdError(fc, total)
	}

	return results, nil
}

func (m *MapState) failureThresholdError(failures, total int) error {
	return fmt.Errorf("States.ExceedToleratedFailureThreshold: %d of %d items failed", failures, total)
}

func (m *MapState) effectiveConcurrency(maxConcurrency, totalItems int) int {
	if maxConcurrency <= 0 {
		return totalItems
	}
	if maxConcurrency > totalItems {
		return totalItems
	}
	return maxConcurrency
}

func (m *MapState) exceedsFailureThreshold(failures, total, toleratedCount int, toleratedPercentage float64) bool {
	if toleratedCount > 0 && failures > toleratedCount {
		return true
	}
	if toleratedPercentage > 0 {
		actualPercentage := (float64(failures) / float64(total)) * 100
		if actualPercentage > toleratedPercentage {
			return true
		}
	}
	// Default: zero tolerance — any failure exceeds the threshold
	if toleratedCount == 0 && toleratedPercentage == 0 && failures > 0 {
		return true
	}
	return false
}

func (m *MapState) executeIteration(ctx context.Context, input interface{}) (interface{}, error) {
	currentStateName := m.itemProcessor.StartAt
	currentInput := input

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		state, exists := m.itemProcessor.States[currentStateName]
		if !exists {
			return nil, fmt.Errorf("state '%s' not found in ItemProcessor", currentStateName)
		}

		output, nextState, err := state.Execute(ctx, currentInput)
		if err != nil {
			return nil, err
		}

		if state.IsEnd() || nextState == nil {
			return output, nil
		}

		currentStateName = *nextState
		currentInput = output
	}
}

// MarshalJSON implements custom JSON marshaling
func (m *MapState) MarshalJSON() ([]byte, error) {
	return MarshalStateWithBase(m.BaseState, struct {
		ItemsPath                      *string                `json:"ItemsPath,omitempty"`
		ItemSelector                   map[string]interface{} `json:"ItemSelector,omitempty"`
		Parameters                     map[string]interface{} `json:"Parameters,omitempty"`
		MaxConcurrency                 *int                   `json:"MaxConcurrency,omitempty"`
		MaxConcurrencyPath             *string                `json:"MaxConcurrencyPath,omitempty"`
		ToleratedFailurePercentage     *float64               `json:"ToleratedFailurePercentage,omitempty"`
		ToleratedFailurePercentagePath *string                `json:"ToleratedFailurePercentagePath,omitempty"`
		ToleratedFailureCount          *int                   `json:"ToleratedFailureCount,omitempty"`
		ToleratedFailureCountPath      *string                `json:"ToleratedFailureCountPath,omitempty"`
		ItemBatcher                    *ItemBatcherConfig     `json:"ItemBatcher,omitempty"`
		ResultSelector                 map[string]interface{} `json:"ResultSelector,omitempty"`
		Retry                          []RetryPolicy          `json:"Retry,omitempty"`
		Catch                          []CatchPolicy          `json:"Catch,omitempty"`
		ItemProcessor                  *ItemProcessor         `json:"ItemProcessor,omitempty"`
	}{
		ItemsPath:                      m.ItemsPath,
		ItemSelector:                   m.ItemSelector,
		Parameters:                     m.Parameters,
		MaxConcurrency:                 m.MaxConcurrency,
		MaxConcurrencyPath:             m.MaxConcurrencyPath,
		ToleratedFailurePercentage:     m.ToleratedFailurePercentage,
		ToleratedFailurePercentagePath: m.ToleratedFailurePercentagePath,
		ToleratedFailureCount:          m.ToleratedFailureCount,
		ToleratedFailureCountPath:      m.ToleratedFailureCountPath,
		ItemBatcher:                    m.ItemBatcher,
		ResultSelector:                 m.ResultSelector,
		Retry:                          m.Retry,
		Catch:                          m.Catch,
		ItemProcessor:                  m.itemProcessor,
	})
}

// toInt converts an interface{} value to int
func toInt(v interface{}) (int, error) {
	switch val := v.(type) {
	case float64:
		if val != math.Trunc(val) {
			return 0, fmt.Errorf("expected integer, got float: %v", val)
		}
		return int(val), nil
	case int:
		return val, nil
	case int64:
		return int(val), nil
	case json.Number:
		i, err := val.Int64()
		if err != nil {
			return 0, err
		}
		return int(i), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}

// toFloat converts an interface{} value to float64
func toFloat(v interface{}) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case json.Number:
		return val.Float64()
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}
