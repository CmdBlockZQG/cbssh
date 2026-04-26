# cbssh

`cbssh` 是一个用 Go 编写的 SSH 连接和 tunnel 管理 CLI。它使用 TOML 保存连接配置，支持通过连接名称递归配置多级跳板，并可以把 SSH tunnel 启动为后台常驻进程。

当前版本是第一版可运行 MVP，已包含：

- SSH 连接配置管理模型
- TOML 配置读取、写入、严格校验
- 明文密码和私钥登录
- Go 原生多级跳板连接
- `local` / `remote` / `dynamic` tunnel 配置
- 后台常驻 tunnel 进程
- tunnel 状态查看和停止
- 基础交互式 TUI 管理菜单

## 构建

```bash
go build -o cbssh ./cmd/cbssh
```

也可以使用 `Makefile`：

```bash
make help
make run ARGS='ls'
make dev ARGS='config validate'
make build VERSION=0.1.0
make dist VERSION=0.1.0
```

`make dev` 使用 `.tmp/cbssh/config.toml` 和 `.tmp/cbssh/state.json`，适合本地调试，不会影响默认用户配置。`make dist` 会编译 Linux 和 macOS 的 `amd64` / `arm64` 二进制到 `dist/`。

## 配置文件

默认配置路径：

```text
~/.config/cbssh/config.toml
```

可以通过命令查看：

```bash
cbssh config path
```

初始化空配置：

```bash
cbssh config init
```

配置示例：

```toml
default_key_path = "~/.ssh/id_ed25519"
host_key_check = "insecure"

[[hosts]]
name = "bastion"
host = "203.0.113.10"
port = 22
user = "ubuntu"
tags = ["prod"]

[hosts.auth]
type = "key"
key_path = "~/.ssh/id_ed25519"

[[hosts]]
name = "prod-db"
host = "10.0.1.20"
port = 22
user = "ubuntu"
jump = "bastion"
tags = ["prod", "db"]

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

`jump` 使用连接名称引用。每个连接只需要配置一个跳板，`cbssh` 会递归解析出完整链路。

## 常用命令

启动 TUI：

```bash
cbssh
cbssh tui
```

列出连接：

```bash
cbssh ls
cbssh ls --sort name
cbssh ls --tag prod
```

查看连接详情：

```bash
cbssh show <name>
```

连接 SSH 终端：

```bash
cbssh connect <name>
cbssh c <name>
```

启动 tunnel：

```bash
cbssh tunnel <name>
cbssh tunnel start <name>
cbssh tunnel start <name> <tun> <tun>
```

查看 tunnel 状态：

```bash
cbssh status
cbssh status <name>
cbssh tunnel status
```

停止 tunnel：

```bash
cbssh stop
cbssh stop <name>
cbssh stop <name> <tun>
cbssh tunnel stop <name> <tun>
```

校验配置：

```bash
cbssh config validate
```

用 `$EDITOR` 打开配置：

```bash
cbssh config edit
```

## Tunnel 类型

`local` 对应 `ssh -L`：

```toml
type = "local"
listen_host = "127.0.0.1"
listen_port = 3307
target_host = "127.0.0.1"
target_port = 3306
```

`remote` 对应 `ssh -R`：

```toml
type = "remote"
listen_host = "127.0.0.1"
listen_port = 8080
target_host = "127.0.0.1"
target_port = 8080
```

`dynamic` 对应 `ssh -D`，提供本地 SOCKS5 代理：

```toml
type = "dynamic"
listen_host = "127.0.0.1"
listen_port = 1080
```

## 状态和日志

Linux 默认状态文件：

```text
~/.local/state/cbssh/state.json
```

macOS 默认状态文件：

```text
~/Library/Application Support/cbssh/state.json
```

日志默认保存在状态文件同级的 `logs` 目录。`cbssh status` 会显示每个活跃 tunnel 的 PID 和日志路径。

## 安全说明

当前版本按需求支持在 TOML 中明文保存密码。建议把配置文件权限设为只有当前用户可读写：

```bash
chmod 600 ~/.config/cbssh/config.toml
```

`cbssh config validate` 会在配置文件对其他用户可读时输出 warning。

`host_key_check` 目前支持：

- `insecure`：默认值，不校验 host key
- `known_hosts`：使用 `~/.ssh/known_hosts` 校验

## TODO

- 每个 tunnel 使用独立后台进程，便于单独停止；后续可以优化成同一连接下多个 tunnel 复用一个 SSH client。

## 已知问题

- **PID 回收导致状态不一致**：系统运行时 PID 上限为 4194304，日常桌面环境绕一圈需要极长时间，实际几乎不可能发生。重启后可通过 `StartedAt` 与系统启动时间对比自动清理。纯运行时 PID 回收场景暂未修复，因为触发概率极低。
