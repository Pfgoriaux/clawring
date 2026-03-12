# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| Latest  | Yes                |
| < Latest| No                 |

Only the latest release receives security updates. We recommend always running the most recent version.

## Reporting a Vulnerability

If you discover a security vulnerability in Clawring, please report it responsibly.

**Do NOT open a public GitHub issue for security vulnerabilities.**

Instead, email **security@openclaw.dev** with:

- A description of the vulnerability
- Steps to reproduce
- The potential impact
- Any suggested fix (optional)

## What Qualifies as a Security Issue

- Authentication or authorization bypass
- Credential leakage (master key, API keys, phantom tokens)
- Encryption weaknesses in the AES-256-GCM implementation
- SQL injection or other injection attacks
- Denial of service vulnerabilities
- Rate limiting bypass
- Any issue that could expose sensitive data

## Response Timeline

- **Acknowledgment**: Within 48 hours of your report
- **Initial assessment**: Within 5 business days
- **Fix timeline**: Depends on severity, but we aim for:
  - Critical: Patch within 7 days
  - High: Patch within 14 days
  - Medium/Low: Patch in the next regular release

## Disclosure

We follow coordinated disclosure. We ask that you give us reasonable time to address the issue before making it public. We will credit reporters in the release notes unless they prefer to remain anonymous.
