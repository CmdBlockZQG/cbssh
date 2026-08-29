# cbssh

[English](README.md)

`cbssh` 是一个精简的后台 SSH 隧道管理器。它直接调用系统 OpenSSH 并使用用户已有的
`~/.ssh/config`。

## 要求

- Linux 或 macOS
- 支持 `-M`、`-S` 以及 `-O forward/cancel/check/exit` 的 OpenSSH 客户端
- Go 1.26.2 或更新版本（仅源码构建需要）

## 配置

隧道清单默认位于 `~/.cbssh/tunnels.toml`。

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
cbssh init
cbssh validate
cbssh list
cbssh ls

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

- `--dir <path>`：指定 cbssh 的配置与运行目录。
- `--ssh-config <path>`：通过 `ssh -F` 指定 OpenSSH 配置；省略时使用系统默认配置链。

## 数据目录与故障

cbssh 将 `tunnels.toml`、`runtime.json`、私有 control socket、锁和日志统一保存在
`~/.cbssh/`。使用 `--dir` 时会整体切换这套目录布局。

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

## 与 v1.0 的区别

v1.0 是由 cbssh 自行保存 SSH 主机与认证配置、并使用内置 Go SSH 客户端的版本，包含
TUI、交互 SSH、SFTP 和后台隧道功能。

当前版本是破坏性重构，旧版 `config.toml` 不能直接用于当前版本。

## 许可证

MIT，详见 [LICENSE](LICENSE)。
