# AGENTS.md — Technical Instructions for bbbserver-cli

This document is authoritative for technical decisions and implementation details.

## Goal
Implement a production-grade CLI for the HTTP API described in `./collection.json` (Postman Collection v2.x). The CLI must be easy to extend as endpoints evolve.

## Language & Runtime
- Go (latest stable supported by common distros; set `go` directive to a modern version, e.g. 1.22+ if available in your environment).
- Must compile and run on Linux `amd64` and `arm64`.
- Build single binaries (static-ish): `CGO_ENABLED=0`.

## CLI Framework
- Use `github.com/spf13/cobra` for commands, help, and shell completions.
- Provide shell completion generation:
  - `bbbserver-cli completion bash`
  - `bbbserver-cli completion zsh`
  - `bbbserver-cli completion fish`
  - `bbbserver-cli completion powershell`

## Configuration & Precedence
Provide the following configuration sources and precedence:

1. CLI flags (highest)
2. Environment variables
3. Config file
4. Defaults (lowest)

### Required Settings
- `base_url` (string)
- `api_key` (string)
- `timeout` (duration)

### Env Vars
- `BBBSERVER_BASE_URL`
- `BBBSERVER_API_KEY`
- `BBBSERVER_TIMEOUT`

### Config File
- Default location: OS config dir (use `os.UserConfigDir()`), e.g.:
  - `~/.config/bbbserver-cli/config.yaml`
- Provide commands:
  - `bbbserver-cli config init`
  - `bbbserver-cli config show` (MUST mask api_key)
  - `bbbserver-cli config set base-url <url>`
  - `bbbserver-cli config set api-key <key>`
- Never print the raw API key in logs or debug output.

### Flags (global)
- `--base-url string`
- `--api-key string`
- `--timeout duration` (default 30s)
- `--output string` (`human` default, `json` alternative)
- `--pretty` (pretty JSON when output=json)
- `--verbose`
- `--debug` (HTTP debug; MUST sanitize secrets)

## HTTP Client Requirements
Implement an API client in `internal/api`:
- `type Client struct { BaseURL string; APIKey string; HTTP *http.Client; UserAgent string; Debug bool }`
- Default auth header: `X-API-Key: <key>`
- Also support optional bearer mode as a config option (for future compatibility), but default to X-API-Key.
- Use `context.Context` + request-scoped timeout.
- Properly join BaseURL + paths; avoid double slashes.
- Parse JSON responses into structs when possible.
- Error handling must map HTTP status codes to structured errors.

## Postman Collection Handling
- Parse `collection.json` to discover requests (name, method, URL path, query params).
- Implement a pragmatic mapping strategy:
  - Create command groups based on folder structure in the collection.
  - Create leaf commands for each request.
- For each request:
  - Support specifying path variables and query params as flags when feasible.
  - Support request bodies (JSON) via:
    - `--data @file.json` to load JSON from a file
    - `--data '{"key":"value"}'` for inline JSON
- If auto-generation is too heavy, implement a smaller subset plus a clear extension pattern; however, the preferred path is to use the Postman collection to drive command creation at runtime OR via a small codegen step included in the repo.

## Output Rules
- `--output human`:
  - Human-readable output to stdout.
  - Use `text/tabwriter` for tables when listing items.
- `--output json`:
  - Output must be strict JSON on stdout (no extra text).
  - If `--pretty`, indent JSON.
- Errors:
  - Print human-readable errors to stderr.
  - If `--output json`, ALSO print a JSON error object to stdout:
    ```json
    {"error":{"message":"...","type":"...","status":400,"request_id":"..."}}
    ```
  - Ensure stdout remains valid JSON in json mode even on failures.

## Exit Codes
- `0` success
- `1` general error
- `2` usage/validation error
- `3` auth error (401/403)
- `4` not found (404)
- `5` server error (>=500)
- `6` network/timeout error

## Commands (minimum)
Even if Postman-driven generation exists, implement these first-class commands:
- `bbbserver-cli version`
- `bbbserver-cli health` (check whether API is reachable / auth doesn't matter)
- `bbbserver-cli me` (show info about the authed user)

## Project Layout
Recommended structure:
- `cmd/bbbserver-cli/` (main)
- `internal/cli/` (cobra root + command wiring)
- `internal/config/` (viper config, masking)
- `internal/api/` (client, errors, request builder)
- `internal/output/` (human + json renderers)
- `internal/postman/` (Postman parsing + command mapping)
- `internal/version/` (Version variable for ldflags)

## Release Engineering
- Provide `Makefile` with:
  - `build`, `test`, `lint` (optional), `install`, `release-dry`
- Provide `.goreleaser.yaml` to build:
  - linux/amd64
  - linux/arm64
- Set version via `-ldflags "-X internal/version.Version=..."`.
- Ensure `go test ./...` passes.

## Security
- Never log API keys.
- Sanitize debug logs:
  - redact headers `X-API-Key`, `Authorization`.
  - redact query params named `api_key`, `token`, etc. (basic heuristic).
- Mask API key in `config show` (e.g., show last 4 characters only).

## Documentation
README.md is authoritative for user-facing behavior and examples. Keep implementation aligned.
