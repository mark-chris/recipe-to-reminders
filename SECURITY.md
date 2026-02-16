# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in this project, please report it privately via [GitHub Security Advisories](https://github.com/mark-chris/recipe-to-reminders/security/advisories/new).

Do not open a public issue for security vulnerabilities.

## Supported Versions

Only the latest version on `main` is supported with security updates.

## Security Measures

- **SSRF protection** on all URL fetching (scheme allowlist, IP blocklist, redirect re-validation)
- **Input validation** with strict size and format limits on all API inputs
- **Dependency scanning** via Dependabot alerts and automated security updates
- **Static analysis** via gosec (SARIF), CodeQL (weekly), and govulncheck in CI
- **Secret scanning** via pre-commit gitleaks hook
- **No secrets in code** — all credentials via environment variables
