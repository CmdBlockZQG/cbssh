# cbssh

`cbssh` 是一个用 Go 编写的 SSH 连接和 tunnel 管理 CLI。它使用 TOML 保存连接配置，支持通过连接名称递归配置多级跳板，并可以把 SSH tunnel 启动为后台常驻进程。

当前版本是第一版可运行 MVP，已包含：

- SSH 连接配置管理模型
- TOML 配置读取、写入、严格校验
- 明文密码和私钥登录
- Go 原生多级跳板连接
- 基于 SFTP 的文件上传和下载
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

[hosts.auth]
type = "key"
key_path = "~/.ssh/id_ed25519"

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
```

查看连接详情：

```bash
cbssh info <name>
```

连接 SSH 终端：

```bash
cbssh connect <name>
cbssh c <name>
```

上传文件或目录：

```bash
cbssh file upload <name> <local> [remote]
cbssh file up <name> <local> [remote]
cbssh file upload <name> ./dist /opt/app --recursive
```

下载文件或目录：

```bash
cbssh file download <name> <remote> [local]
cbssh file down <name> <remote> [local]
cbssh file download <name> /var/log/app ./logs --recursive
```

顶层 `upload` / `up` / `download` / `down` 是对应 `file` 子命令的快捷别名，例如 `cbssh up prod ./app.tar.gz` 等价于 `cbssh file upload prod ./app.tar.gz`。

文件传输默认不会覆盖已有文件，使用 `--force` 覆盖。目录传输需要显式加 `--recursive`。命令每次都会新建 SSH/SFTP 会话，不会记录上一次传输或浏览时的远端目录状态。远端相对路径和 `~/path` 都基于该次 SFTP 会话的远端初始目录解析，通常是远端登录用户的 home 目录。

远端路径以 `~` 开头时需要加引号或转义，例如 `'~/app.log'` 或 `\~/app.log`。如果直接写 `~/app.log`，本地 shell 会在 `cbssh` 启动前把它展开成本机用户的 home 路径，程序收到的会是类似 `/home/local-user/app.log` 的远端绝对路径。

省略最后一个路径参数时会使用源路径的 basename：上传 `./app.tar.gz` 会写到远端初始目录下的 `app.tar.gz`，上传 `./dist --recursive` 会写到远端初始目录下的 `dist/`；下载 `/var/log/app.log` 会保存为本地当前目录下的 `app.log`，下载 `/tmp/release --recursive` 会保存为本地当前目录下的 `release/`。

启动 tunnel：

```bash
cbssh tunnel <name>
cbssh tunnel start <name>
cbssh tunnel start <name> [<tun>, <tun>, ...]
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

日志默认保存在状态文件同级的 `logs` 目录。`cbssh status` 会显示每个活跃 tunnel 的 PID 和日志路径。同一主机下的多个 tunnel 会优先复用同一个后台 daemon 和 SSH client，因此可能显示相同 PID。

状态文件会记录后台进程身份指纹。清理状态和停止 tunnel 时，程序会校验 PID 是否仍然对应同一个 `cbssh daemon`，避免 PID 被系统回收或重启后误把其他进程当作活跃 tunnel，或误杀无关进程。

## 安全说明

当前版本按需求支持在 TOML 中明文保存密码。建议把配置文件权限设为只有当前用户可读写：

```bash
chmod 600 ~/.config/cbssh/config.toml
```

`cbssh config validate` 会在配置文件对其他用户可读时输出 warning。

`host_key_check` 目前支持：

- `insecure`：默认值，不校验 host key
- `known_hosts`：使用 `~/.ssh/known_hosts` 校验
