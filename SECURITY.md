# Security Policy

## Supported Versions

We release patches for security vulnerabilities. Which versions are eligible for receiving such patches depend on the CVSS v3.0 Rating:

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

We take the security of our software seriously. If you believe you have found a security vulnerability, please report it to us as described below.

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, please report them via email to the maintainers. You should receive a response within 48 hours. If for some reason you do not, please follow up via email to ensure we received your original message.

Please include the requested information listed below (as much as you can provide) to help us better understand the nature and scope of the possible issue:

- Type of issue (e.g. buffer overflow, SQL injection, cross-site scripting, etc.)
- Full paths of source file(s) related to the manifestation of the issue
- The location of the affected source code (tag/branch/commit or direct URL)
- Any special configuration required to reproduce the issue
- Step-by-step instructions to reproduce the issue
- Proof-of-concept or exploit code (if possible)
- Impact of the issue, including how an attacker might exploit it

This information will help us triage your report more quickly.

## Preferred Languages

We prefer all communications to be in English.

## What to Expect

After you submit a vulnerability report, we will:

1. **Acknowledge** - We will acknowledge receipt of your vulnerability report within 3 business days
2. **Investigate** - We will investigate and validate the issue
3. **Update** - We will provide an estimated timeline for a fix
4. **Fix** - We will work on a fix and keep you updated on our progress
5. **Release** - We will release a security patch and credit you for the discovery

## Security Best Practices

When using this library, please follow these security best practices:

### 1. Input Validation
- Always validate and sanitize input data before processing
- Use appropriate data types for input parameters
- Implement input length and format checks

### 2. Authentication & Authorization
- Implement proper authentication for workflow execution
- Use role-based access control for workflow management
- Validate permissions before executing sensitive operations

### 3. Data Protection
- Encrypt sensitive data in transit and at rest
- Implement proper logging without exposing sensitive information
- Use secure configuration management

### 4. Resource Management
- Implement rate limiting for workflow execution
- Set appropriate timeouts for long-running workflows
- Monitor resource usage and implement limits

### 5. Dependency Security
- Keep dependencies up to date
- Regularly audit dependencies for known vulnerabilities
- Use verified and trusted packages only

## Security Features

This library includes the following security features:

### Input Validation
- JSON/YAML schema validation
- Path expression validation
- State machine definition validation

### Resource Control
- Execution timeouts
- Memory usage limits
- Concurrent execution limits

### Safe Execution
- Sandboxed task execution (when using local executor)
- Error isolation between workflow executions
- Graceful failure handling

## Known Security Considerations

### 1. JSON/XML External Entity (XXE) Protection
The parser is configured to reject external entity references to prevent XXE attacks.

### 2. Path Injection Protection
Path expressions are validated to prevent directory traversal attacks.

### 3. Resource Exhaustion Protection
Built-in limits prevent resource exhaustion attacks through infinite loops or excessive recursion.

### 4. Code Injection Protection
When using the local executor with custom handlers, ensure handlers are properly sanitized and validated.

## Updates and Patches

Security updates will be released as patches to supported versions. We recommend:

1. Always using the latest stable version
2. Subscribing to security announcements
3. Regularly updating dependencies
4. Monitoring for security advisories

## Credits

We would like to thank all security researchers who responsibly disclose vulnerabilities. Contributors will be credited in release notes unless they wish to remain anonymous.

## Contact

For security-related issues, please contact: hussainpithawala

**Note**: Replace the email address above with the appropriate contact for your project.

---

*Last updated: December 2025*
