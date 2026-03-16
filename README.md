# bbbserver-cli

`bbbserver-cli` is a command-line tool for managing your [bbbserver](https://app.bbbserver.de) account. It gives you direct access to the bbbserver HTTP API from your terminal — manage conference rooms, users, recordings, customer settings, and more without leaving the command line.

## Download & Install

Download the pre-built binary for your platform from the [`dist/`](dist/) folder in this repository:

| Platform | Architecture | Download |
|----------|-------------|----------|
| Linux | x86_64 (amd64) | [bbbserver-cli-linux-amd64](https://github.com/invokablegmbh/bbbserver-cli/raw/master/dist/bbbserver-cli-linux-amd64) |
| Linux | ARM64 | [bbbserver-cli-linux-arm64](https://github.com/invokablegmbh/bbbserver-cli/raw/master/dist/bbbserver-cli-linux-arm64) |
| Windows | x86_64 (amd64) | [bbbserver-cli-windows-amd64.exe](https://github.com/invokablegmbh/bbbserver-cli/raw/master/dist/bbbserver-cli-windows-amd64.exe) |

No installation required — the download is a standalone binary. Just make it executable and run it:

```bash
chmod +x bbbserver-cli-linux-amd64
./bbbserver-cli-linux-amd64 --help
```

Optionally, move it into your PATH for convenience:

```bash
sudo mv bbbserver-cli-linux-amd64 /usr/local/bin/bbbserver-cli
```

## Authentication

`bbbserver-cli` authenticates using your **System API key**.

### Getting your API key

If you don't have an API key yet:

1. Log in to your bbbserver account at <https://app.bbbserver.de>
2. Go to **Profile** -> **API credentials (SystemAPI)**
3. Click **Manage my API key** -> **Generate new api key**
4. Copy the generated API key

### Configuring authentication

The recommended way is to store your API key in a config file:

```bash
bbbserver-cli config init
bbbserver-cli config set api-key YOUR_API_KEY
```

Alternatively, set it via environment variable:

```bash
export BBBSERVER_API_KEY="YOUR_API_KEY"
```

Or pass it directly as a flag (not recommended for regular use, as the key may appear in shell history):

```bash
bbbserver-cli --api-key YOUR_API_KEY me
```

## Usage

### Getting help

Every command and subcommand has a detailed `--help` page — this is the best way to discover what's available and how each command works:

```bash
bbbserver-cli --help
bbbserver-cli conference-rooms --help
bbbserver-cli conference-rooms list --help
```

### Quick start

Check that the API is reachable:

```bash
bbbserver-cli health
```

Show your account info:

```bash
bbbserver-cli me
```

List all available commands:

```bash
bbbserver-cli list
```

### Working with commands

Commands are organized into groups that mirror the bbbserver API. Some examples:

```bash
# List conference rooms
bbbserver-cli conference-rooms list

# Get details for a specific conference room
bbbserver-cli conference-rooms get --conferenceid 12345

# List moderators
bbbserver-cli moderators list

# List recordings
bbbserver-cli recordings list
```

### Sending data

For commands that create or update resources, provide a JSON body with `--data`:

```bash
# Inline JSON
bbbserver-cli conference-rooms create --data '{"name":"My Room"}'

# From a file
bbbserver-cli conference-rooms create --data @room.json
```

Some commands (e.g. file uploads) use dedicated flags instead:

```bash
bbbserver-cli customer-settings branding-logo-set --logo /path/to/logo.png
```

### JSON output

By default, output is human-readable. Use `--output json` for machine-readable JSON, e.g. for scripting:

```bash
bbbserver-cli --output json conference-rooms list
bbbserver-cli --output json --pretty me
```

## Configuration

Configuration can be set through (in order of priority):

1. **CLI flags** (highest priority)
2. **Environment variables**
3. **Config file**
4. **Defaults** (lowest priority)

### Config file

The config file is stored at `~/.config/bbbserver-cli/config.yaml` (Linux).

```bash
bbbserver-cli config init          # Create a default config file
bbbserver-cli config show          # Show current configuration (API key is masked)
bbbserver-cli config set base-url https://app.bbbserver.de/bbb-system-api
bbbserver-cli config set api-key YOUR_API_KEY
```

### Environment variables

| Variable | Description |
|----------|-------------|
| `BBBSERVER_BASE_URL` | API base URL |
| `BBBSERVER_API_KEY` | API key |
| `BBBSERVER_TIMEOUT` | Request timeout (e.g. `30s`) |

## Global Flags

These flags are available on every command:

| Flag | Default | Description |
|------|---------|-------------|
| `--base-url` | `https://app.bbbserver.de/bbb-system-api` | API base URL |
| `--api-key` | — | API key for authentication |
| `--timeout` | `30s` | Request timeout |
| `--output` | `human` | Output format: `human` or `json` |
| `--pretty` | `false` | Pretty-print JSON output (only with `--output json`) |
| `--verbose` | `false` | Enable verbose output |
| `--debug` | `false` | Show HTTP request/response details (secrets are redacted) |

## Shell Completion

Generate auto-completion scripts for your shell:

```bash
bbbserver-cli completion bash
bbbserver-cli completion zsh
bbbserver-cli completion fish
bbbserver-cli completion powershell
```

For example, to enable bash completion permanently:

```bash
bbbserver-cli completion bash | sudo tee /etc/bash_completion.d/bbbserver-cli > /dev/null
```
