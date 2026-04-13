# Release Notes v1.2.20

**Release Date**: 2026-04-13

## Overview

Version 1.2.20 focuses on performance optimization and repository organization. This release introduces a composite index for linked executions to significantly improve query performance, along with a major reorganization of documentation for better maintainability.

## What's New

### 🚀 Performance Improvements

#### Composite Index for Linked Executions
- Added `idx_linked_exec_composite` index on `linked_executions` table
- Dramatically improves performance for filtered `NOT EXISTS` queries
- Optimizes execution filtering in `ListNonLinkedExecutions` queries
- Better correlated subquery performance for production workloads

### 📁 Project Organization

#### Documentation Restructure
- Moved all release notes to dedicated `releases/` folder
- Moved example documentation to `example-docs/` folder
- Cleaner root directory for better project navigation
- Improved maintainability and file discoverability

### 🧹 Code Cleanup

- Removed binary artifacts from version control
- Cleaned up example compilation binaries
- Removed temporary database message files
- Updated release script to reflect new folder structure
- General code quality improvements

## Migration Guide

### Database Changes

**Required**: Add the composite index for optimal linked execution query performance:

```sql
CREATE INDEX IF NOT EXISTS idx_linked_exec_composite 
ON linked_executions (state_machine_id, execution_id, source_state_name);
```

**Note**: Verify the exact columns in your implementation before creating the index.

### Breaking Changes

None. This release is fully backward-compatible.

## Technical Details

### Changes Summary
- **Files Modified**: 58 files
- **Lines Added**: 27
- **Lines Removed**: 18
- **Net Change**: +9 lines (excluding file moves)

### Key Commits
- `63ffdfa` - feat(indexing): add composite index for linked executions to optimize NOT EXISTS queries
- `47a04ea` - general cleanup
- `d4047b8` - move releases to a dedicated folder

## Performance Impact

### Query Performance
- **Linked Execution Filtering**: Significantly faster with composite index
- **NOT EXISTS Queries**: Optimized for correlated subquery patterns
- **Execution List Operations**: Improved filtering performance

### Repository Structure
- Better organization reduces onboarding time for new contributors
- Clearer separation between release documentation and example documentation
- Easier maintenance and navigation

## Upgrade Path

### From v1.2.19

1. Pull the latest code:
   ```bash
   git pull origin master
   ```

2. Run the database migration to add the composite index:
   ```sql
   CREATE INDEX IF NOT EXISTS idx_linked_exec_composite 
   ON linked_executions (state_machine_id, execution_id, source_state_name);
   ```

3. Verify the index was created:
   ```sql
   \di idx_linked_exec_composite
   ```

4. No application code changes required

## Compatibility

- **Backward Compatible**: ✅ Yes
- **Database Migration**: ⚠️ Recommended (index addition)
- **API Changes**: ❌ None
- **Configuration Changes**: ❌ None

## Bug Fixes

No functional bug fixes in this release. Focus is on performance and organization.

## Documentation

- All release notes now located in `releases/` directory
- Example documentation moved to `example-docs/` directory
- Updated changelog with v1.2.20 entries

## Checklist for Production Deployment

- [ ] Review database migration script
- [ ] Backup database before applying index
- [ ] Test index creation in staging environment
- [ ] Monitor query performance after deployment
- [ ] Update deployment documentation with new folder structure

## Contributors

Thank you to all contributors who made this release possible!

---

**Full Changelog**: [v1.2.19...v1.2.20](https://github.com/your-org/state-machine-amz-go/compare/v1.2.19...v1.2.20)
