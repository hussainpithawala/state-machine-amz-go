# Release Notes - v1.2.2

## 📝 Documentation Cleanup

**Release Date**: February 5, 2026

### Overview

Version 1.2.2 is a documentation maintenance release that cleans up the README.md to remove historical version-specific references and issue notes, providing a cleaner and more streamlined documentation experience for new users.

### What's Changed

#### README.md Cleanup

**Changes Made:**

1. **Removed version-specific sections** - Eliminated all "What's New" and "Previous Release" sections (v1.2.1, v1.2.0, v1.1.9, v1.1.8, etc.)
2. **Removed version references** - Cleaned up phrases like "(NEW in v1.1.0)", "(v1.0.8)", etc. throughout the documentation
3. **Streamlined roadmap** - Removed version numbers from completed roadmap items
4. **Simplified feature descriptions** - Focused on current capabilities without historical context

**Files Modified:**
- `README.md`

**Lines Removed:** 177 lines of version-specific content

**Why This Change Matters:**

The README now provides a cleaner, more professional presentation that:
- Focuses on what the library can do today, not historical bug fixes
- Reduces noise for new users trying to understand capabilities
- Maintains better documentation sustainability (less maintenance burden)
- Keeps historical release notes in dedicated files for those who need them

### Example Changes

**Before:**
```markdown
## 🆕 What's New in v1.2.1

**🐛 Bug Fix** - Message state timeout input preservation fixed!
...
[Long section about specific bug fix]

---

## 🔄 Previous Release - v1.2.0
...
[Multiple previous release sections]
```

**After:**
```markdown
## ✨ Features

- 🚀 **AWS Step Functions Compatible** - Use identical state definitions...
- 💾 **Persistent Execution** - Built-in PostgreSQL persistence...
[Direct focus on features]
```

### Impact

**Severity**: Documentation Only

**Users Affected**: None - purely documentation changes

**Breaking Changes**: None

**Action Required**: None - all historical release notes remain available in dedicated `RELEASE_NOTES_v*.md` files

### Benefits

1. **🎯 Cleaner Documentation** - New users see capabilities without historical noise
2. **📚 Better Onboarding** - Focus on features and usage, not bug history
3. **🔄 Easier Maintenance** - Less duplication between README and release notes
4. **📖 Historical Tracking** - Release notes still available in dedicated files

### Release History

All historical release notes remain available:
- `RELEASE_NOTES_v1.2.1.md` - Message state timeout input preservation
- `RELEASE_NOTES_v1.2.0.md` - Comprehensive failure testing
- `RELEASE_NOTES_v1.1.9.md` - Timeout trigger generation fix
- `RELEASE_NOTES_v1.1.8.md` - State-specific correlation keys
- And all previous versions...

### Recommendation

**Severity**: Documentation Maintenance

**Action**: No action required - purely documentation improvements

**Safe to Upgrade**: Yes, no functional changes

---

**Full Changelog**: https://github.com/hussainpithawala/state-machine-amz-go/compare/v1.2.1...v1.2.2

**Report Issues**: https://github.com/hussainpithawala/state-machine-amz-go/issues

**Questions?** Open a discussion: https://github.com/hussainpithawala/state-machine-amz-go/discussions
