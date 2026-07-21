# Security Policy

## Reporting a vulnerability

Report authentication bypasses, SSRF, unsafe URL handling, cache poisoning, or secret exposure through [GitHub Security Advisories](https://github.com/starcat-app/starcat-wiki-api/security/advisories/new). Do not publish API keys, database contents, probed private URLs, or production logs in an issue.

Security fixes target the current default branch and latest deployed version. Runtime secrets must be injected through environment variables or Fly.io secrets and must never be committed.
