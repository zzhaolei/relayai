# RelayAI

AI API 代理服务器，支持 Claude、Codex 请求转发。

## 前置依赖

- Go 1.26+
- Node.js 18+

安装工具依赖：
```bash
go install tool
```

## 构建

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
> ```bash
> wails3 task setup:docker
> ```
>
> **规则**：macOS 目标只能用 macOS 或 Docker；Linux 目标需要 Linux 原生或 Docker；Windows 任意主机均可 Go 原生交叉编译。

## 运行

```bash
# macOS（.app 包）
open bin/RelayAI.app

# macOS（直接运行二进制）
./bin/RelayAI

# Windows
bin\RelayAI.exe

# Linux
./bin/relayai
```

## 使用

1. 添加 Provider（名称、URL、API Key）
2. 点击"写入配置"将代理地址写入 CLI 配置
3. 启动代理
4. 直接使用 CLI 工具即可

## 开发

```bash
wails3 dev
```

## 数据存储

`~/.relayai/relayai.db`

## 许可证

MIT
