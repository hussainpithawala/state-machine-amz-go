# Release Notes - v1.2.8

## Overview

Version 1.2.8 focuses on refactoring and enhancing the execution payload system, improving the handling of input transformers and uniqueness constraints for state machine executions.

## What's Changed

### Refactored Input Transformer Handling

- **Cleaner Architecture**: Removed redundant inline logic for input transformer name assignment, resulting in more maintainable code.
- **Enhanced Payload Structure**: Added `InputTransformerName` and `ApplyUnique` fields to `ExecutionTaskPayload` for better execution control.
- **Improved Task Processing**: Updated execution task creation and processing flows to properly handle the new payload fields.
- **Configuration Enhancement**: State machine execution configuration now includes first-class support for input transformers and uniqueness constraints.

## Benefits

- **Better Code Maintainability**: Refactored logic reduces duplication and improves code clarity.
- **Enhanced Flexibility**: New payload fields provide more control over execution behavior.
- **Improved Consistency**: Unified handling of input transformers across the execution pipeline.

## Upgrade Notes

This release maintains backward compatibility. No migration or configuration changes are required.

---

**Full Changelog**: v1.2.7...v1.2.8
