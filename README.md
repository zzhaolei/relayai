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
# macOS Apple Silicon
wails build -platform darwin/arm64

# macOS Intel
wails build -platform darwin/amd64

# Windows 64-bit
wails build -platform windows/amd64

# Windows ARM64
wails build -platform windows/arm64

# Linux 64-bit
wails build -platform linux/amd64

# Linux ARM64
wails build -platform linux/arm64
```

## 运行

```bash
# macOS
open build/bin/RelayAI.app

# Windows
build\bin\RelayAI.exe

# Linux
./build/bin/relayai
```

## 使用

1. 添加 Provider（名称、URL、API Key）
2. 点击"写入配置"将代理地址写入 CLI 配置
3. 启动代理
4. 直接使用 CLI 工具即可

## 开发

```bash
wails dev
```

## 数据存储

`~/.relayai/relayai.db`

## 许可证

MIT
