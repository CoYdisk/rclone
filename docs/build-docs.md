# rclone 交叉编译打包脚本

## 概述

`build-rclone.py` 是一个跨平台的 rclone 交叉编译打包脚本，可在 **Windows、Linux、macOS** 上运行（需 Python 3.7+）。

通过配置 `GOOS`/`GOARCH` 环境变量调用 `go build`，将 rclone 编译为指定平台的二进制文件。

---

## 环境要求

- **Python** 3.7 或更高版本
- **Go** 1.18 或更高版本（用于编译 rclone）
- **rclone 源码目录**（脚本需在项目根目录运行）

---

## 快速开始

```bash
# 克隆 rclone 源码（如未拉取）
git clone https://github.com/rclone/rclone.git
cd rclone

# 默认打包：linux/amd64 + linux/arm64 + windows/386 + darwin/amd64
python build-rclone.py
```

产物默认输出到 `build/` 目录。

---

## 命令行参数

| 参数 | 简写 | 说明 | 默认值 |
|------|------|------|--------|
| `--targets` | `-t` | 目标平台列表，空格分隔，格式 `os/arch` | `linux/amd64 linux/arm64 windows/386 darwin/amd64` |
| `--out-dir` | `-o` | 产物输出目录 | `build` |
| `--version` | `-v` | 版本号；为空时自动从 `VERSION` 文件读取 | 自动 |
| `--tags` | — | Go 构建标签（build tags）；设为 `cmount` 可启用 FUSE 挂载支持 | `cmount` |
| `--strip` | — | 剥离调试符号（减小体积） | 启用 |
| `--no-strip` | — | 保留调试符号（二进制更大） | — |
| `--verbose` | `-V` | 显示详细构建命令和输出 | 关闭 |
| `--list-targets` | — | 列出所有支持的目标平台后退出 | — |
| `--help` | `-h` | 显示帮助信息 | — |

---

## 使用示例

### 1. 默认打包（4 个常用平台）

```bash
python build-rclone.py
```

产物：
| 文件 | 平台 |
|------|------|
| `rclone-linux-amd64` | Linux x86_64 |
| `rclone-linux-arm64` | Linux ARM64 |
| `rclone-windows-386.exe` | Windows 32-bit (x86) |
| `rclone-darwin-amd64` | macOS Intel |

### 2. 自定义目标平台

```bash
# 只打包 Linux 和 Windows 的 64 位版本
python build-rclone.py --targets linux/amd64 windows/amd64

# 打包到自定义目录
python build-rclone.py --targets linux/amd64 linux/arm64 darwin/arm64 --out-dir dist

# 打包所有支持的平台（共 29 个）
python build-rclone.py --targets $(python build-rclone.py --list-targets | grep -oP '^\s+\K\S+' | tr '\n' ' ')
```

### 3. 保留调试符号

```bash
# 二进制体积更大，但可用 gdb/lldb 调试
python build-rclone.py --no-strip
```

大小对比（以 `linux/amd64` 为例）：

| 模式 | 约计大小 | 说明 |
|------|---------|------|
| `--strip`（默认） | 76 MB | `-ldflags "-s -w"`，剥离符号表和 DWARF |
| `--no-strip` | 105 MB | 无额外 ldflags，保留完整调试信息 |

### 4. 指定版本号

```bash
# 手动指定版本（覆盖 VERSION 文件）
python build-rclone.py --version v1.75.0-beta

# 版本号会写入 rclone 的 fs.Version 变量
# 可通过 rclone version 命令查看
```

### 5. 使用构建标签

> **注意**：从 v7.3.3 起，`cmount` 标签已改为默认启用（`--tags cmount`），
> 确保 macOS/Windows 的 FUSE 挂载功能正常。如需禁用可显式指定 `--tags ""`。

```bash
# cmount 已默认启用，通常无需手动指定
python build-rclone.py

# 如需覆盖默认标签
python build-rclone.py --tags ""

# 多个标签用空格分隔
python build-rclone.py --tags "cmount nocrypto"
```

### 6. 详细输出

```bash
python build-rclone.py --verbose
```

会显示每次构建的完整 `go build` 命令和环境变量。

---

## 支持的目标平台

运行以下命令查看所有支持的 `os/arch` 组合：

```bash
python build-rclone.py --list-targets
```

输出示例：

```
Supported target platforms (os/arch):

  windows/386                 [GO386=softfloat]
  windows/amd64
  windows/arm64
  darwin/amd64
  darwin/arm64
  linux/386                   [GO386=softfloat]
  linux/amd64
  linux/arm                   [GOARM=5]
  linux/arm-v6                [GOARM=6]
  linux/arm-v7                [GOARM=7]
  linux/arm64
  ...
```

方括号内为架构特有的环境变量（如 `GOARM=5`、`GO386=softfloat`），脚本会自动设置。

完整列表（共 29 个，来源 `bin/cross-compile.go`）：

| 操作系统 | 架构 |
|----------|------|
| aix | ppc64 |
| darwin | amd64, arm64 |
| freebsd | 386, amd64, arm, arm-v6, arm-v7 |
| linux | 386, amd64, arm, arm-v6, arm-v7, arm64, mips, mipsle |
| netbsd | 386, amd64, arm, arm-v6, arm-v7 |
| openbsd | 386, amd64 |
| plan9 | 386, amd64 |
| solaris | amd64 |
| windows | 386, amd64, arm64 |

---

## 产物说明

构建完成后，脚本会打印产物列表及大小。产物命名规则：

```
rclone-{os}-{arch}[.exe]
```

- Windows 平台自动添加 `.exe` 后缀
- 其他平台为无后缀的 ELF/Mach-O 可执行文件

示例：

```
Artifacts:
  rclone-darwin-amd64                          77.2 MB
  rclone-linux-amd64                           75.7 MB
  rclone-linux-arm64                           69.4 MB
  rclone-windows-386.exe                       71.2 MB
```

---

## 常见问题

### Q: 编译时提示 "go: not found"

确保 Go 已安装并添加到 `PATH`。验证方法：

```bash
go version
```

### Q: macOS 编译后 rclone 没有 mount 命令

检查 `rclone version` 输出中的 `go/tags` 字段：

- `go/tags: cmount` ✅ 正常，mount 功能已启用
- `go/tags: none` ❌ 缺少 mount 支持

原因：macOS 上 `mount` 命令通过 `cmd/cmount/` 实现，需要 `cmount` 构建标签 + `cgo` 才能编译进去。

确认 `build-rclone.py` 的默认 `--tags` 为 `cmount`（查看文件顶部参数默认值）。

macOS 编译还需安装 macFUSE SDK 头文件：

```bash
# macOS 上安装 macFUSE（会自动安装 SDK 头文件）
brew install --cask macfuse

# 交叉编译时需要 C 交叉编译工具链（如 osxcross）
```

验证构建标签是否生效：

```bash
python build-rclone.py --verbose --targets darwin/amd64 | grep "tags"
```

### Q: 编译速度慢或内存不足

Go 交叉编译需要较多内存。可减少并行度（Go 本身不支持，可一次只打包一个目标）：

```bash
python build-rclone.py --targets linux/amd64
python build-rclone.py --targets linux/arm64
# ...
```

### Q: 产物在目标平台无法运行

确认目标平台的架构匹配。例如 ARM Linux 设备需确认是 `arm`（32位）还是 `arm64`（64位）：

```bash
uname -m
# armv7l → 用 linux/arm 或 linux/arm-v7
# aarch64 → 用 linux/arm64
```

### Q: 如何交叉编译 CGO 依赖的功能（如 cmount）

核心 rclone 是纯 Go 编译的，无需 CGO。如需 cmount 等 CGO 功能，需在目标平台上原生编译，或在当前平台安装对应的交叉编译工具链。

---

## 相关文件

| 文件 | 说明 |
|------|------|
| `build-rclone.py` | 打包脚本 |
| `build-rclone-docs.md` | 本文档 |
| `bin/cross-compile.go` | rclone 官方交叉编译脚本（Go 版） |
| `Makefile` | rclone 构建命令（`make cross`、`make beta`） |
| `VERSION` | 当前版本号 |
