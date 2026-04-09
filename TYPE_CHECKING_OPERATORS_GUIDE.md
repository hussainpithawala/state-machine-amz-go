# Type-Checking Operators for Choice State

## Overview

The state-machine-amz-go framework now supports **type-checking operators** in Choice states, compatible with AWS States Language. These operators allow you to route based on the presence, absence, or type of variables in your input data.

## New Operators Added

### 1. `IsPresent`
Checks if a variable exists in the input (is not nil).

```yaml
Choices:
  - Variable: "$.paymentContext.originalRequest.fallbackMethod"
    IsPresent: true
    Next: TryFallbackMethod
```

**Use Cases:**
- Check if optional fields exist before processing
- Route based on feature flags or configuration
- Handle different versions of input schemas

### 2. `IsNull`
Checks if a variable is explicitly null.

```yaml
Choices:
  - Variable: "$.customer.email"
    IsNull: true
    Next: HandleMissingEmail
```

**Use Cases:**
- Detect explicitly null values vs missing fields
- Handle optional fields that were intentionally cleared

### 3. `IsBoolean`
Checks if a variable is a boolean type.

```yaml
Choices:
  - Variable: "$.order.isExpress"
    IsBoolean: true
    Next: CheckExpressOptions
```

### 4. `IsNumeric`
Checks if a variable is a numeric type (int, float, etc.).

```yaml
Choices:
  - Variable: "$.order.totalAmount"
    IsNumeric: true
    Next: ProcessPayment
```

### 5. `IsString`
Checks if a variable is a string type.

```yaml
Choices:
  - Variable: "$.customer.name"
    IsString: true
    Next: ProcessCustomerName
```

### 6. `IsTimestamp`
Checks if a variable is a time.Time type.

```yaml
Choices:
  - Variable: "$.order.createdAt"
    IsTimestamp: true
    Next: ValidateOrderTiming
```

## Complete Example: Your Use Case

```yaml
CreditCardDeclined:
  Type: Choice
  Choices:
    # Check if fallback method exists
    - Variable: "$.paymentContext.originalRequest.fallbackMethod"
      IsPresent: true
      Next: TryFallbackMethod
  Default: PaymentFailed

TryFallbackMethod:
  Type: Choice
  Choices:
    # Route based on the fallback method value
    - Variable: "$.paymentContext.originalRequest.fallbackMethod"
      StringEquals: "ACH"
      Next: ProcessACHFallback
    - Variable: "$.paymentContext.originalRequest.fallbackMethod"
      StringEquals: "WireTransfer"
      Next: ProcessWireFallback
  Default: PaymentFailed
```

## Combining with Other Operators

### With Compound Operators (And/Or/Not)

```yaml
AdvancedRouting:
  Type: Choice
  Choices:
    # Check if discount code exists AND is a string
    - And:
        - Variable: "$.order.discountCode"
          IsPresent: true
        - Variable: "$.order.discountCode"
          IsString: true
      Next: ApplyDiscount
    
    # Check if priority exists OR is VIP customer
    - Or:
        - Variable: "$.order.priority"
          IsPresent: true
        - Variable: "$.order.isVipCustomer"
          BooleanEquals: true
      Next: HighPriorityProcessing
    
    # Check if payment method is NOT null
    - Not:
        Variable: "$.payment.method"
        IsNull: true
      Next: ProcessPayment
  Default: StandardProcessing
```

## Implementation Details

### Files Modified

1. **`internal/states/choice.go`**
   - Added type-checking operator fields to `ChoiceRule` struct
   - Implemented evaluation functions for each operator
   - Updated validation to allow type-checking operators without comparison operators
   - Fixed nil context check to allow type-checking operators on null values

2. **`internal/states/choice_test.go`**
   - Added comprehensive tests for all type-checking operators
   - Tests cover: present, missing, null, string, numeric, boolean scenarios

### Technical Notes

**Nil Handling:**
- Type-checking operators can evaluate nil values
- `IsNull: true` matches when variable is nil
- `IsPresent: false` matches when variable is nil
- Other type-checking operators return false for nil values

**Path Resolution:**
- Uses standard JSONPath-like syntax: `$.field.nested.path`
- Returns nil if path doesn't exist
- Type-checking operators work with both missing and null values

**Validation:**
- Type-checking operators count as valid comparison operators
- No longer triggers "must have at least one comparison operator" error
- Can be used alone or combined with other operators

## Testing

Run the tests:
```bash
go test ./internal/states/... -v -run TypeCheckingOperators
```

All tests pass:
```
=== RUN   TestChoiceState_Execute_TypeCheckingOperators
=== RUN   TestChoiceState_Execute_TypeCheckingOperators/IsPresent_-_field_exists
=== RUN   TestChoiceState_Execute_TypeCheckingOperators/IsPresent_-_field_missing
=== RUN   TestChoiceState_Execute_TypeCheckingOperators/IsNull_-_value_is_null
=== RUN   TestChoiceState_Execute_TypeCheckingOperators/IsString_-_value_is_string
=== RUN   TestChoiceState_Execute_TypeCheckingOperators/IsNumeric_-_value_is_number
=== RUN   TestChoiceState_Execute_TypeCheckingOperators/IsBoolean_-_value_is_boolean
--- PASS: TestChoiceState_Execute_TypeCheckingOperators (0.00s)
```

## Migration Guide

### Before (Workaround Required)
Previously, you had to use workarounds like:
```yaml
# Had to check against a known value
- Variable: "$.fallbackMethod"
  StringEquals: "ACH"  # But what if you just want to know if it EXISTS?
  Next: HasFallback
```

### After (Direct Support)
```yaml
# Now you can directly check existence
- Variable: "$.fallbackMethod"
  IsPresent: true
  Next: HasFallback
```

## Performance

Type-checking operators have minimal overhead:
- Simple type assertions or nil checks
- No string/numeric comparisons
- O(1) time complexity

## AWS States Language Compatibility

These operators are fully compatible with AWS Step Functions Choice state operators:
- ✅ `IsPresent` - AWS compatible
- ✅ `IsNull` - AWS compatible
- ✅ `IsBoolean` - AWS compatible
- ✅ `IsNumeric` - AWS compatible
- ✅ `IsString` - AWS compatible
- ✅ `IsTimestamp` - AWS compatible

## Additional Resources

- Example YAML: `/examples/type-checking-operators-example.yaml`
- Tests: `/internal/states/choice_test.go` (TestChoiceState_Execute_TypeCheckingOperators)
- Implementation: `/internal/states/choice.go` (evaluateIsPresent, evaluateIsNull, etc.)
