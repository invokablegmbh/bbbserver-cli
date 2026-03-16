# bbbserver-cli

`bbbserver-cli` is a single-binary command-line client for the BBBServer SaaS HTTP API. It reads its API surface from the Postman collection at build time and embeds it into the binary.

## Features

- **Single binary** for Linux `amd64` and `arm64`
- **Config precedence**: flags > env > config file > defaults
- **Human output** (default) or **strict JSON output**
- **Shell completion** for bash/zsh/fish/powershell
- **Build-time embedded API surface** from Postman collection

## Installation

### From Release Artifact
Download the appropriate binary for your system and put it in your PATH:

```bash
chmod +x bbbserver-cli
sudo mv bbbserver-cli /usr/local/bin/bbbserver-cli
````

### Build from Source

Requirements:

* Go toolchain installed

```bash
make build
ls -lh dist/
```

`make build` always produces a full cross-platform build in `dist/`:

* `bbbserver-cli-linux-amd64`
* `bbbserver-cli-linux-arm64`
* `bbbserver-cli-windows-amd64.exe`

## Configuration

### 1) Config file (recommended)

Initialize the config file:

```bash
bbbserver-cli config init
```

Set values:

```bash
bbbserver-cli config set base-url https://app.bbbserver.de/bbb-system-api
bbbserver-cli config set api-key YOUR_API_KEY
```

Show current config (API key is masked):

```bash
bbbserver-cli config show
```

Config file location:

* Linux typically: `~/.config/bbbserver-cli/config.yaml`

### 2) Environment variables

* `BBBSERVER_BASE_URL`
* `BBBSERVER_API_KEY`
* `BBBSERVER_TIMEOUT` (e.g. `30s`)

Example:

```bash
export BBBSERVER_BASE_URL="https://app.bbbserver.de/bbb-system-api"
export BBBSERVER_API_KEY="YOUR_API_KEY"
```

### 3) Flags (highest priority)

Global flags:

* `--base-url string` (default: `https://app.bbbserver.de/bbb-system-api`)
* `--api-key string`
* `--timeout duration` (default: `30s`)
* `--output human|json` (default: `human`)
* `--pretty` (pretty JSON; only affects `--output json`)
* `--verbose`
* `--debug` (sanitized HTTP debug)

Example:

```bash
bbbserver-cli --base-url https://app.bbbserver.de/bbb-system-api --api-key "$BBBSERVER_API_KEY" health
```

## Usage

### Help

```bash
bbbserver-cli --help
bbbserver-cli <command> --help
```

### Health Check

```bash
bbbserver-cli health
```

### Current User / Account

```bash
bbbserver-cli me
```

### Collection-driven commands

Commands are derived from the folder/request structure in the embedded Postman collection. Run:

```bash
bbbserver-cli list
```

…to see available commands, or use:

```bash
bbbserver-cli --help
```

The collection is always downloaded during build/test/install and embedded into the binary. No runtime collection URL or file is required.

## Output Formats

### Human output (default)

Human-readable output is printed to stdout. Tables may be used for list responses.

```bash
bbbserver-cli me
```

### JSON output

Strict JSON is printed to stdout:

```bash
bbbserver-cli --output json me
```

Pretty JSON:

```bash
bbbserver-cli --output json --pretty me
```

### Error output in JSON mode

* Human-readable error text is printed to **stderr**
* A JSON error object is printed to **stdout** (so stdout remains valid JSON)

Example shape:

```json
{
  "error": {
    "message": "Unauthorized",
    "type": "auth_error",
    "status": 401,
    "request_id": "..."
  }
}
```

## Request Bodies (for POST/PUT/PATCH)

For endpoints requiring a JSON body, use `--data`:

* Inline JSON:

  ```bash
  bbbserver-cli <command> --data '{"name":"example"}'
  ```

* From file:

  ```bash
  bbbserver-cli <command> --data @payload.json
  ```

## File Uploads

Some endpoints (e.g. branding presentation, branding logo, slide uploads) accept file uploads. These commands expose per-field flags instead of `--data`:

```bash
bbbserver-cli customer-settings branding-presentation-set --slides /path/to/presentation.pdf
bbbserver-cli customer-settings branding-logo-set --logo /path/to/logo.png
bbbserver-cli conference-rooms upload-slides --slides /path/to/slides.pdf --conferenceid <id>
```

## Shell Completion

Generate completion scripts:

```bash
bbbserver-cli completion bash
bbbserver-cli completion zsh
bbbserver-cli completion fish
bbbserver-cli completion powershell
```

### Bash (example)

```bash
bbbserver-cli completion bash | sudo tee /etc/bash_completion.d/bbbserver-cli > /dev/null
```

### Zsh (example)

```zsh
bbbserver-cli completion zsh > "${fpath[1]}/_bbbserver-cli"
autoload -Uz compinit && compinit
```

### Fish (example)

```bash
bbbserver-cli completion fish > ~/.config/fish/completions/bbbserver-cli.fish
```

### PowerShell (example)

```powershell
bbbserver-cli completion powershell | Out-String | Invoke-Expression
```

## Debugging

Enable sanitized HTTP debug output:

```bash
bbbserver-cli --debug me
```

Notes:

* API keys are always redacted.
* Sensitive headers and common token query parameters are sanitized.

## Exit Codes

* `0` success
* `1` general error
* `2` usage/validation error
* `3` auth error (401/403)
* `4` not found (404)
* `5` server error (>=500)
* `6` network/timeout error

## Development

Common tasks:

```bash
make collection-generate
make build
make test
```

Release (dry run):

```bash
make release-dry
```

## Repository Layout

* `cmd/bbbserver-cli/` — main entrypoint
* `internal/cli/` — cobra commands
* `internal/postman/` — Postman collection parsing + command mapping
* `internal/api/` — HTTP client + errors
* `internal/config/` — config handling
* `internal/output/` — human/json output formatting
* `internal/version/` — version injection
