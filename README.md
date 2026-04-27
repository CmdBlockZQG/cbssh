# cbssh

[中文](README.zh-CN.md)

> SSH connection and tunnel manager — configure once, connect anywhere.

`cbssh` manages SSH hosts, multi-hop jump chains, file transfers over SFTP, and long-running
SSH tunnels (local, remote, dynamic) from a single TOML config file. Built in Go with no
runtime dependencies.

## Features

- Terminal-based TUI for browsing hosts and managing tunnels
- Multi-hop SSH connections with recursive jump resolution
- SFTP file upload/download with recursive and force-overwrite support
- Interactive remote file browser (TUI)
- Long-running background tunnels (local, remote, SOCKS5 dynamic)
- TOML configuration with validation and in-editor editing
- Key-based and password authentication, with SSH agent support

## Quick Start

```bash
# Install
go install github.com/cmdblock/cbssh/cmd/cbssh@latest

# Create default config
cbssh config init

# Edit config to add your hosts
cbssh config edit

# Launch the dashboard
cbssh
```

## Commands

### Dashboard

| Command | Description |
|---|---|
| `cbssh` | Open the interactive TUI (host list, tunnels, file browser) |
| `cbssh tui` | Same as above (explicit) |

### Hosts

| Command | Description |
|---|---|
| `cbssh ls [-s recent\|name]` | List all configured hosts (`-s` / `--sort`) |
| `cbssh info <name>` | Show host details (address, user, jump chain, tunnels) |
| `cbssh connect <name>` | Open an interactive SSH session |

### File Transfer

| Command | Description |
|---|---|
| `cbssh file upload <name> <local> [remote]` | Upload file or directory via SFTP |
| `cbssh file download <name> <remote> [local]` | Download file or directory via SFTP |
| `cbssh file tui <name>` | Browse remote files in interactive TUI |

File transfer flags: `-r, --recursive` (directories), `-f, --force` (overwrite), `-q, --quiet`.

### Tunnels

| Command | Description |
|---|---|
| `cbssh tunnel start <name> [tunnel...]` | Start default or specified tunnels |
| `cbssh tunnel stop [name] [tunnel...]` | Stop active tunnels |
| `cbssh tunnel status [name]` | List active tunnels |
| `cbssh tunnel restart <name> [tunnel...]` | Restart tunnels (stop then start) |

Omitting tunnel names on `start` launches all default tunnels; on `stop` stops **all** active
tunnels for the host regardless of the `default` flag. Omitting the host name on `stop`/`status`
acts across all hosts.

### Configuration

| Command | Description |
|---|---|
| `cbssh config path` | Print the config file path |
| `cbssh config init` | Create an empty config if missing |
| `cbssh config validate` | Validate config syntax and file permissions |
| `cbssh config edit` | Open config in `$EDITOR` (falls back to `vi`) |

### Shortcuts

The following top-level commands are aliases for
`file` and `tunnel` subcommands:

| Shortcut | Equivalent |
|---|---|
| `cbssh c <name>` | `cbssh connect <name>` |
| `cbssh up <name> <local> [remote]` | `cbssh file upload <name> <local> [remote]` |
| `cbssh down <name> <remote> [local]` | `cbssh file download <name> <remote> [local]` |
| `cbssh browse <name>` | `cbssh file tui <name>` |
| `cbssh status [name]` | `cbssh tunnel status [name]` |
| `cbssh stop [name] [tunnel...]` | `cbssh tunnel stop [name] [tunnel...]` |
| `cbssh start <name> [tunnel...]` | `cbssh tunnel start <name> [tunnel...]` |

## Configuration

Default config path is platform-dependent:

| OS | Path |
|---|---|
| Linux | `~/.config/cbssh/config.toml` |
| macOS | `~/Library/Application Support/cbssh/config.toml` |

Pass `--config` and `--state` flags to override defaults (persistent flags available on all
commands).

### Complete Example

```toml
default_key_path = "~/.ssh/id_ed25519"
host_key_check = "insecure"

[[hosts]]
name = "bastion"
host = "203.0.113.10"
port = 22
user = "ubuntu"

[hosts.auth]
type = "key"
key_path = "~/.ssh/id_ed25519"
# passphrase = "optional-key-passphrase"
# use_agent = true

[[hosts]]
name = "prod-db"
host = "10.0.1.20"
port = 22
user = "ubuntu"
jump = "bastion"

[hosts.auth]
type = "password"
password = "plain-text-password"

[[hosts.tunnels]]
name = "mysql"
type = "local"
listen_host = "127.0.0.1"
listen_port = 3307
target_host = "127.0.0.1"
target_port = 3306
default = true

[[hosts.tunnels]]
name = "socks"
type = "dynamic"
listen_host = "127.0.0.1"
listen_port = 1080
default = false
```

`jump` references another host by name. `cbssh` recursively resolves the full chain —
a host can jump through a bastion that itself jumps through another host.

`port` defaults to `22` if omitted. `default_key_path` defaults to `~/.ssh/id_ed25519`
when not set; it is also the fallback when an individual host omits `key_path`.

### Auth Fields

| Field | Required | Description |
|---|---|---|
| `type` | No | `key` or `password`; auto-detected from other fields if omitted |
| `password` | For `password` | Plain-text password |
| `key_path` | No | Path to the private key file; falls back to `default_key_path` |
| `passphrase` | No | Passphrase for encrypted private keys |
| `use_agent` | No | Use `SSH_AUTH_SOCK` agent for key authentication |

### Tunnel Types

| Type | Equivalent | Requires |
|---|---|---|
| `local` | `ssh -L` | `target_host`, `target_port` |
| `remote` | `ssh -R` | `target_host`, `target_port` |
| `dynamic` | `ssh -D` | — (SOCKS5 proxy on `listen_host:listen_port`) |

### Tunnel Fields

| Field | Required | Default | Description |
|---|---|---|---|
| `name` | Yes | — | Tunnel identifier |
| `type` | No | `local` | `local`, `remote`, or `dynamic` |
| `listen_host` | No | `127.0.0.1` | Local listener address |
| `listen_port` | Yes | — | Local listener port |
| `target_host` | For `local`/`remote` | — | Target server address |
| `target_port` | For `local`/`remote` | — | Target server port |
| `default` | No | `false` | Start this tunnel when no names are given to `tunnel start` |

### Host Key Checking

| Value | Behavior |
|---|---|
| `insecure` | Skip host key verification (default) |
| `known_hosts` | Verify against `~/.ssh/known_hosts` |
| `known-hosts` | Same as `known_hosts` |

## Installation

```bash
# Build locally
make build                  # → bin/cbssh

# Cross-compile (linux/darwin × amd64/arm64)
make dist                   # → dist/

# Test
make test
make vet

# Dev mode (uses .tmp/cbssh/ for config/state)
make dev-init
make dev ARGS='ls'
```

## Internals

### Multi-hop

Jump chains are built with Go's `golang.org/x/crypto/ssh` — no external `ssh` binary or
`ProxyJump` directive. Each hop opens an SSH connection and creates a `net.Conn` dialer
through it, resolving the full chain sequentially.

### State & daemons

Active tunnel processes run as detached daemons managed through a JSON state file.
On startup, `cbssh` verifies the identity of any PID recorded in the state to prevent
stale entries from conflicting with reused PIDs.

| OS | State file |
|---|---|
| Linux | `~/.local/state/cbssh/state.json` |
| macOS | `~/Library/Application Support/cbssh/state.json` |

Tunnel logs are written to `logs/` next to the state file.

### Security

- Passwords are stored as plaintext in the TOML config — set restrictive file permissions:
  `chmod 600 ~/.config/cbssh/config.toml`
- `cbssh config validate` warns if the config file is readable by others.
- The daemon tunnel command is hidden (`cbssh daemon tunnel`) and not meant for direct use.

## License

MIT — see [LICENSE](LICENSE).
