# RelayAI

将多厂商 AI 模型订阅转换为 Claude / Codex 可用接口。

## 前置依赖

- Go 1.26+
- Node.js 18+
- [Wails v3](https://wails.io/docs/next/guides/installation)（用于构建）

安装工具依赖：

```bash
go install tool
```

## 快速开始

```bash
# 查看所有可用命令
make help

# 安装依赖
make install

# 启动开发模式（热更新）
make dev
```

## 构建

### 使用 Make（推荐）

```bash
# 构建当前平台（macOS 自动打包为 .app，其他平台生成可执行文件）
make build

# 构建并运行
make run

# 构建指定平台
make build-darwin         # macOS .app 包
make build-windows        # Windows 可执行文件
make build-linux          # Linux 可执行文件

# 构建指定架构
make build-darwin-arm64   # macOS Apple Silicon
make build-darwin-amd64   # macOS Intel
make build-darwin-universal # macOS Universal（arm64 + amd64）

# 其他命令
make clean                # 清理构建产物
make test                 # 运行测试
make lint                 # 代码检查
make info                 # 显示构建信息
```

### 使用 Wails3（高级）

```bash
# macOS — 产出 .app 包
wails3 task darwin:package                # 当前架构
wails3 task darwin:package ARCH=arm64     # Apple Silicon，需 macOS 或 Docker
wails3 task darwin:package ARCH=amd64     # Intel，需 macOS 或 Docker
wails3 task darwin:package:universal      # Universal（arm64+amd64）

# Windows — 产出 .exe 二进制
wails3 task windows:build ARCH=amd64      # 任意主机均可交叉编译
wails3 task windows:build ARCH=arm64

# Linux — 产出二进制
wails3 task linux:build ARCH=amd64        # 需 Linux 原生或 Docker
wails3 task linux:build ARCH=arm64
```

> 跨 OS 构建前准备 Docker 镜像（一次性）：
>
> ```bash
> make setup-docker
> # 或
> wails3 task setup:docker
> ```
>
> **规则**：macOS 目标只能用 macOS 或 Docker；Linux 目标需要 Linux 原生或 Docker；Windows 任意主机均可 Go 原生交叉编译。

## 运行

```bash
# 构建并运行（推荐）
make run

# 或手动运行
# macOS
open bin/RelayAI.app

# Windows
bin\RelayAI.exe

# Linux
./bin/RelayAI
```

## 开发

```bash
# 启动开发模式（推荐）
make dev

# 或直接使用 wails3
wails3 dev

# 仅启动前端开发服务器
make dev-frontend
```

## 使用

1. 添加 Provider（名称、URL、API Key）
2. 点击"写入配置"将代理地址写入 CLI 配置
3. 启动代理
4. 直接使用 CLI 工具即可

## 数据存储

`~/.relayai/relayai.db`

## 许可证

MIT
