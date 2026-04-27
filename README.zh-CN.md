# cbssh

[English](README.md)

> SSH 连接与隧道管理器 —— 一次配置，随处连接。

`cbssh` 通过单个 TOML 配置文件管理 SSH 主机、多级跳板链、SFTP 文件传输以及长期运行
的 SSH 隧道（local / remote / dynamic）。使用 Go 编写，编译后为无外部依赖的单文件。

## 特性

- 基于终端的 TUI 界面，管理主机和隧道
- 多级 SSH 跳板，支持链式递归解析
- SFTP 文件上传/下载，支持递归目录和强制覆盖
- 交互式远端文件浏览器（TUI）
- 长期运行的后台隧道（local, remote, SOCKS5 dynamic）
- TOML 配置，支持语法校验和编辑器内编辑
- 支持密钥和密码认证，以及 SSH Agent

## 快速开始

```bash
# 安装
go install github.com/cmdblock/cbssh/cmd/cbssh@latest

# 创建默认配置
cbssh config init

# 编辑配置，添加你的主机
cbssh config edit

# 启动仪表盘
cbssh
```

从源码安装需要 Go 1.26.2 或更新版本。预编译二进制不需要安装 Go。

## 命令

### 仪表盘

| 命令 | 说明 |
|---|---|
| `cbssh` | 打开交互式 TUI（主机列表、隧道、文件浏览） |
| `cbssh tui` | 同上（显式调用） |

### 主机管理

| 命令 | 说明 |
|---|---|
| `cbssh ls [-s recent\|name]` | 列出所有已配置的主机（`-s` / `--sort`） |
| `cbssh info <name>` | 查看主机详细信息 |
| `cbssh connect <name>` | 打开交互式 SSH 会话 |

### 文件传输

| 命令 | 说明 |
|---|---|
| `cbssh file upload <name> <local> [remote]` | 通过 SFTP 上传文件或目录 |
| `cbssh file download <name> <remote> [local]` | 通过 SFTP 下载文件或目录 |
| `cbssh file tui <name>` | 在交互式 TUI 中浏览远端文件 |

文件传输通用选项：`-r, --recursive`（目录传输）、`-f, --force`（覆盖）、`-q, --quiet`。

### 隧道管理

| 命令 | 说明 |
|---|---|
| `cbssh tunnel start <name> [tunnel...]` | 启动默认或指定的隧道 |
| `cbssh tunnel stop [name] [tunnel...]` | 停止活跃隧道 |
| `cbssh tunnel status [name]` | 查看活跃隧道 |

`start` 省略隧道名称时启动所有标记为默认的隧道；`stop` 省略隧道名称时停止该主机
**所有**活跃隧道（不区分 `default` 标记）。`stop` / `status` 省略主机名称时作用于所有主机。

### 配置管理

| 命令 | 说明 |
|---|---|
| `cbssh config path` | 打印配置文件的路径 |
| `cbssh config init` | 若配置不存在则创建空配置 |
| `cbssh config validate` | 校验配置语法和文件权限 |
| `cbssh config edit` | 用 `$EDITOR` 打开配置文件（回退到 `vi`） |

### 快捷方式

以下顶层命令是 `file` 和 `tunnel`
子命令的别名：

| 快捷方式 | 等效命令 |
|---|---|
| `cbssh c <name>` | `cbssh connect <name>` |
| `cbssh up <name> <local> [remote]` | `cbssh file upload <name> <local> [remote]` |
| `cbssh down <name> <remote> [local]` | `cbssh file download <name> <remote> [local]` |
| `cbssh browse <name>` | `cbssh file tui <name>` |
| `cbssh status [name]` | `cbssh tunnel status [name]` |
| `cbssh start <name> [tunnel...]` | `cbssh tunnel start <name> [tunnel...]` |
| `cbssh stop [name] [tunnel...]` | `cbssh tunnel stop [name] [tunnel...]` |

## 配置

配置文件默认路径因平台而异：

| 操作系统 | 路径 |
|---|---|
| Linux | `~/.config/cbssh/config.toml` |
| macOS | `~/Library/Application Support/cbssh/config.toml` |

可通过 `--config` 和 `--state` 选项覆盖默认路径（这两个是全局选项，所有命令均可用）。

### 完整示例

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
# passphrase = "可选的密钥密码"
# use_agent = true

[[hosts]]
name = "prod-db"
host = "10.0.1.20"
port = 22
user = "ubuntu"
jump = "bastion"

[hosts.auth]
type = "password"
password = "明文密码"

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

`jump` 通过主机名称引用跳板。`cbssh` 递归解析完整链路 —— 一台主机可以通过跳板机连接，
跳板机自身也可以再通过其他主机跳转。

`port` 省略时默认为 `22`。`default_key_path` 未设置时默认为
`~/.ssh/id_ed25519`；同时它也是单个主机省略 `key_path` 时的回退值。

### 认证字段

| 字段 | 必填 | 说明 |
|---|---|---|
| `type` | 否 | `key` 或 `password`；省略时根据其他字段自动推断 |
| `password` | 密码认证时 | 明文密码 |
| `key_path` | 否 | 私钥文件路径；空缺时使用 `default_key_path` |
| `passphrase` | 否 | 加密私钥的密码 |
| `use_agent` | 否 | 使用 `SSH_AUTH_SOCK` 代理进行密钥认证 |

### 隧道类型

| 类型 | 等价命令 | 需填 |
|---|---|---|
| `local` | `ssh -L` | `target_host`, `target_port` |
| `remote` | `ssh -R` | `target_host`, `target_port` |
| `dynamic` | `ssh -D` | —（在 `listen_host:listen_port` 启动 SOCKS5 代理） |

### 隧道字段

| 字段 | 必填 | 默认值 | 说明 |
|---|---|---|---|
| `name` | 是 | — | 隧道标识符 |
| `type` | 否 | `local` | `local`、`remote` 或 `dynamic` |
| `listen_host` | 否 | `127.0.0.1` | 监听地址；`local`/`dynamic` 为本地，`remote` 为远端 |
| `listen_port` | 是 | — | 监听端口；`local`/`dynamic` 为本地，`remote` 为远端 |
| `target_host` | `local`/`remote` 时 | — | 目标服务器地址 |
| `target_port` | `local`/`remote` 时 | — | 目标服务器端口 |
| `default` | 否 | `false` | 使用 `tunnel start` 时若未指定隧道名则启动此隧道 |

### Host Key 校验

| 值 | 行为 |
|---|---|
| `insecure` | 跳过 host key 校验（默认） |
| `known_hosts` | 使用 `~/.ssh/known_hosts` 进行校验 |
| `known-hosts` | 等价于 `known_hosts` |

## 安装

```bash
# 本地编译
make build                  # → bin/cbssh

# 交叉编译 (linux/darwin × amd64/arm64)
make dist                   # → dist/

# 测试
make test
make vet

# 开发模式（使用 .tmp/cbssh/ 存放配置和状态文件）
make dev-init
make dev ARGS='ls'
```

## 内部机制

### 多级跳板

跳板链完全基于 Go 的 `golang.org/x/crypto/ssh` 构建 —— 不使用外部 `ssh` 命令或
`ProxyJump` 指令。每一跳打开一条 SSH 连接并通过该连接创建 `net.Conn` 拨号器，
按顺序递归完成整个链路。

### 状态与守护进程

活跃的隧道进程以后台守护进程方式运行，通过 JSON 状态文件管理。
启动时 `cbssh` 会校验状态中记录的 PID 的身份，防止过时条目与系统回收的 PID 冲突。

| 操作系统 | 状态文件路径 |
|---|---|
| Linux | `~/.local/state/cbssh/state.json` |
| macOS | `~/Library/Application Support/cbssh/state.json` |

隧道日志写入状态文件同级的 `logs/` 目录。

### 安全

- 密码以明文形式存储在 TOML 配置文件中 —— 请设置严格的权限：
  `chmod 600 ~/.config/cbssh/config.toml`
- `cbssh config validate` 会在配置文件对其他用户可读时发出警告。
- 守护进程隧道命令为隐藏命令（`cbssh daemon tunnel`），不适合直接使用。

## 许可证

MIT — 详见 [LICENSE](LICENSE)。
