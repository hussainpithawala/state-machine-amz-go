# Release Notes - v1.1.4 (Bug Fix)

## 🐛 Bug Fix

**JSONPath Array Handling** - Fixed a bug in JSONPath processing where `[]map[string]interface{}` array types were not properly handled during value extraction.

## The Issue

In the `getValue` method (`internal/states/jsonpath.go`), the JSONPath processor only handled `[]interface{}` array types when extracting values from array indices. This caused failures when working with `[]map[string]interface{}` array structures, which are common in Go applications.

### Problematic Data Structure
```go
input := map[string]interface{}{
    "accounts": []map[string]interface{}{
        {
            "createdBy": "LESSPAY",
            "bankInfo": map[string]interface{}{
                "bankCode": "100000",
                "bankName": "UNKNOWN BANK",
            },
        },
    },
}
```

### Failing JSONPath Expression
```
$.accounts[0].createdBy
```

This would fail with error: `"cannot index non-array"` even though `accounts` is indeed an array.

### Code Location
**File**: `internal/states/jsonpath.go`
**Method**: `getValue`

## The Fix

Enhanced the array indexing logic to handle both `[]interface{}` and `[]map[string]interface{}` array types:

```go
arr, ok := current.([]interface{})
if !ok {
    // NEW: Handle []map[string]interface{} arrays
    arrOfMaps, ok2 := current.([]map[string]interface{})
    if index < 0 || index >= len(arrOfMaps) {
        return nil, fmt.Errorf("array index out of bounds")
    }
    if !ok2 {
        return nil, fmt.Errorf("cannot index non-array")
    }
    current = arrOfMaps[index]
    continue
} else {
    // Existing: Handle []interface{} arrays
    if index < 0 || index >= len(arr) {
        return nil, fmt.Errorf("array index out of bounds")
    }
    current = arr[index]
    continue
}
```

## Impact

### Who is Affected?
- Users working with **Choice States** that use JSONPath expressions with array indexing
- Workflows that process data structures with `[]map[string]interface{}` arrays
- Applications that use JSONPath to extract values from nested array structures
- Any workflow using `InputPath`, `OutputPath`, `ResultPath`, or `Parameters` with array indexing

### Symptoms Before Fix
- Choice state conditions fail to evaluate correctly
- Error: `"cannot index non-array"` when accessing array elements
- Workflow execution fails or takes wrong conditional branches
- JSONPath expressions with array indices produce errors

### After Fix
- ✅ Both `[]interface{}` and `[]map[string]interface{}` arrays work correctly
- ✅ Choice state conditions evaluate properly
- ✅ Array indexing works with all common Go array types
- ✅ JSONPath expressions handle nested structures correctly

## Use Cases

This fix enables proper handling of common data structures such as:

```yaml
# Choice state with array element access
RouteByAccountType:
  Type: Choice
  Choices:
    - Variable: "$.accounts[0].createdBy"
      StringEquals: "LESSPAY"
      Next: "ProcessLESSPAY"
    - Variable: "$.accounts[0].status"
      StringEquals: "ACTIVE"
      Next: "ProcessActive"
  Default: "DefaultHandler"
```

## Files Changed
- `internal/states/jsonpath.go` - Enhanced array type handling (21 lines modified)
- `internal/states/choice_test.go` - Added test case for `[]map[string]interface{}` arrays (38 lines added)

## Testing

This fix includes comprehensive test coverage:
- ✅ New test case: `"array element access"` with `[]map[string]interface{}`
- ✅ Existing test case: Updated description for `[]interface{}` clarity
- ✅ Validates nested structure access: `$.accounts[0].createdBy`
- ✅ Validates array bounds checking
- ✅ All existing JSONPath tests continue to pass

## Recommended Action

Update to v1.1.4 if you:
- Use Choice states with JSONPath array indexing
- Work with `[]map[string]interface{}` data structures
- Experience JSONPath evaluation errors with arrays

### Update Steps
```bash
go get github.com/hussainpithawala/state-machine-amz-go@v1.1.4
```

### Verification
After updating:
1. Test workflows with Choice states using array indexing
2. Verify JSONPath expressions with `[N]` syntax work correctly
3. Check workflows that process nested array structures
4. Validate conditional branching logic

## Version History
- **v1.1.4** - JSONPath array handling for `[]map[string]interface{}`
- **v1.1.3** - Critical fix for state input propagation
- **v1.1.2** - ExecutionContext moved to types package
- **v1.1.1** - Async task cancellation when messages arrive
- **v1.1.0** - Distributed queue execution with Redis

---

**Note**: This is a bug fix release that improves JSONPath compatibility with common Go data structures. Recommended for all users working with array-based data in their workflows.
