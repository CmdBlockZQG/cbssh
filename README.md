# cbssh

[简体中文](README.zh-CN.md)

`cbssh` is a small background SSH tunnel manager. It invokes the system OpenSSH
client and uses the user's existing `~/.ssh/config`.

## Requirements

- Linux or macOS
- An OpenSSH client supporting `-M`, `-S`, and `-O forward/cancel/check/exit`
- Go 1.26.2 or newer when building from source

## Configuration

Default manifest paths:

| Platform | Path |
|---|---|
| Linux | `~/.config/cbssh/tunnels.toml` |
| macOS | `~/Library/Application Support/cbssh/tunnels.toml` |

Define the connection in OpenSSH first:

```sshconfig
Host prod
    HostName 203.0.113.10
    User ubuntu
    IdentityFile ~/.ssh/id_ed25519
    ProxyJump bastion
    ServerAliveInterval 30
```

Then define the managed forwards in `tunnels.toml`:

```toml
version = 1

[[tunnels]]
name = "prod-db"
host = "prod"
type = "local"
bind_host = "127.0.0.1"
bind_port = 15432
target_host = "127.0.0.1"
target_port = 5432

[[tunnels]]
name = "prod-socks"
host = "prod"
type = "dynamic"
bind_host = "127.0.0.1"
bind_port = 1080
```

`type` must be `local`, `remote`, or `dynamic`. `bind_host` defaults to
`127.0.0.1`, and ports must be between 1 and 65535.

cbssh starts its Master with `ClearAllForwardings=yes`, so `LocalForward`,
`RemoteForward`, and `DynamicForward` entries in SSH config are not imported.
All other OpenSSH behavior, including authentication, host key checking,
`Include`, `Match`, `ProxyJump`, and `ProxyCommand`, remains in effect.

## Commands

```bash
cbssh config init
cbssh config validate
cbssh list
cbssh ls

cbssh start prod-db prod-socks
cbssh start --all
cbssh status
cbssh stop prod-db
cbssh stop --all
cbssh restart prod-db
cbssh logs prod-db
```

`start` and `stop` require names or an explicit `--all`. Both operations are
idempotent. The shared Master exits only after its final tunnel stops.
Restarting one tunnel continues to reuse the existing Master. After changing
SSH config, use `restart --all`, or stop every tunnel for that Host before
starting them again.

Global options:

- `--config <path>` selects the cbssh tunnel manifest.
- `--ssh-config <path>` passes an alternate OpenSSH config with `ssh -F`.

## Runtime State

Runtime state, private control sockets, and logs live under:

| Platform | State directory |
|---|---|
| Linux | `~/.local/state/cbssh/`, or `$XDG_STATE_HOME/cbssh/` |
| macOS | `~/Library/Application Support/cbssh/` |

Directories use mode `0700`; state and log files use `0600`. `status` checks
the OpenSSH control socket and removes stale state. Connections are not
automatically restarted after a network failure; inspect `logs` and run
`restart`.

## Build

```bash
make build
make test
make test-race
make vet
make dist
```

## Differences from v1.0

v1.0 stored SSH hosts and authentication settings itself and used a built-in
Go SSH client. It had TUI, interactive SSH, SFTP, and background tunnels.

The current version is a breaking rewrite; the old `config.toml`
cannot be used directly.

## License

MIT. See [LICENSE](LICENSE).
