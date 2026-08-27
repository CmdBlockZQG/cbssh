# cbssh

[English](README.md)

`cbssh` 是一个精简的后台 SSH 隧道管理器。它不保存主机、用户、密钥或跳板配置，
而是直接调用系统 OpenSSH 并使用用户已有的 `~/.ssh/config`。

同一个 SSH Host 的多条隧道会共享一个由 cbssh 管理的 OpenSSH Master 连接。

## 要求

- Linux 或 macOS
- 支持 `-M`、`-S` 以及 `-O forward/cancel/check/exit` 的 OpenSSH 客户端
- Go 1.26.2 或更新版本（仅源码构建需要）

## 配置

默认配置路径：

| 系统 | 路径 |
|---|---|
| Linux | `~/.config/cbssh/tunnels.toml` |
| macOS | `~/Library/Application Support/cbssh/tunnels.toml` |

先在 OpenSSH 配置中定义连接：

```sshconfig
Host prod
    HostName 203.0.113.10
    User ubuntu
    IdentityFile ~/.ssh/id_ed25519
    ProxyJump bastion
    ServerAliveInterval 30
```

再在 `tunnels.toml` 中定义需要后台管理的转发：

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
name = "prod-api"
host = "prod"
type = "remote"
bind_host = "127.0.0.1"
bind_port = 18080
target_host = "127.0.0.1"
target_port = 8080

[[tunnels]]
name = "prod-socks"
host = "prod"
type = "dynamic"
bind_host = "127.0.0.1"
bind_port = 1080
```

`type` 只能是 `local`、`remote` 或 `dynamic`。`bind_host` 省略时默认为
`127.0.0.1`。端口范围为 `1..65535`。

cbssh 会以 `ClearAllForwardings=yes` 启动自己的 Master，因此 SSH config 中已有的
`LocalForward`、`RemoteForward` 和 `DynamicForward` 不会被带入。连接、认证、Host Key、
`Include`、`Match`、`ProxyJump` 和 `ProxyCommand` 等其他 OpenSSH 设置仍正常生效。

## 命令

```bash
cbssh config init
cbssh config validate
cbssh list

cbssh start prod-db prod-socks
cbssh start --all

cbssh status
cbssh status prod-db

cbssh stop prod-db
cbssh stop --all
cbssh restart prod-db

cbssh logs prod-db
```

`start` 和 `stop` 必须给出名称或显式使用 `--all`。重复启动或停止是幂等的。
同一 Host 的最后一条隧道停止后，共享 Master 才会退出。
单独 `restart` 一条隧道会继续复用同 Host 的现有 Master；修改 SSH config 后请使用
`restart --all`，或先一次性停止该 Host 的全部隧道再重新启动。

可用全局选项：

- `--config <path>`：指定 cbssh 隧道清单。
- `--ssh-config <path>`：通过 `ssh -F` 指定 OpenSSH 配置；省略时使用系统默认配置链。

## 状态与故障

运行状态、私有 control socket 和日志位于系统状态目录：

| 系统 | 状态目录 |
|---|---|
| Linux | `~/.local/state/cbssh/`，或 `$XDG_STATE_HOME/cbssh/` |
| macOS | `~/Library/Application Support/cbssh/` |

目录权限为 `0700`，状态与日志文件权限为 `0600`。`status` 会通过 OpenSSH control
socket 检查连接，并清理失效状态。网络中断后不会自动重连；请查看 `logs` 并执行
`restart`。

## 构建

```bash
make build
make test
make test-race
make vet
make dist
```

## 从旧版升级

这是破坏性重写。旧版 `config.toml`、TUI、交互 SSH、SFTP 和自建 Go SSH daemon
均不再支持。请把主机连接配置迁移到 OpenSSH config，并把需要的隧道写入新的
`tunnels.toml`。

## 许可证

MIT，详见 [LICENSE](LICENSE)。
