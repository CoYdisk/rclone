# rclone WebDAV 后端错误通知增强 — 规格文档

## 1. 概述

### 1.1 目标
在 WebDAV 服务端返回 HTTP 错误（403 权限拒绝、423 文件锁定、507 空间不足等）时，给用户提供更清晰的提示信息，包括终端输出和桌面通知两种方式。

### 1.2 修改范围
- `fs/fs.go` — 新增 sentinel error
- `lib/notify/notify.go` — 新文件，桌面通知模块
- `backend/webdav/webdav.go` — 多项增强
- `go.mod` / `go.sum` — 新增 beeep 依赖

---

## 2. 新增文件

### 2.1 `lib/notify/notify.go`

**路径**: `lib/notify/notify.go`

**用途**: 提供跨平台桌面通知能力，封装 beeep 库。

**API**:
```go
// Send 发送普通桌面通知（没有声音）
func Send(title, message string)

// SendAlert 发送紧急桌面通知（带声音）
func SendAlert(title, message string)
```

**特性**:
- 通知失败时静默处理（仅 DEBUG 日志记录），不影响主流程
- 跨平台：Windows Toast/托盘气泡、Linux D-Bus/notify-send、macOS osascript
- 使用 beeep 库实现，无 CGO 依赖

**依赖**: `github.com/gen2brain/beeep`

---

## 3. 修改文件

### 3.1 `fs/fs.go`

**修改**: 新增 sentinel error

```go
ErrorQuotaExceeded = errors.New("storage quota exceeded")
```

用于 507 Insufficient Storage 错误的语义映射。

### 3.2 `backend/webdav/webdav.go`

#### 3.2.1 新增配置选项

在 `Options` 结构体和注册信息中新增：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `DesktopNotify` | `bool` | `true` | 启用/禁用桌面通知 |

```go
DesktopNotify bool `config:"desktop_notify"`
```

#### 3.2.2 新增导入

```go
"github.com/rclone/rclone/lib/notify"
```

#### 3.2.3 新增方法: `handleHTTPError`

```go
func (f *Fs) handleHTTPError(ctx context.Context, err error, remote string, op string) error
```

**位置**: 紧接 `shouldRetry` 方法之后。

**行为**:

| HTTP 状态码 | 终端输出 | 桌面通知 | 返回的错误 |
|-------------|----------|----------|-----------|
| 403 Forbidden | `fs.Errorf` (ERROR 级别) | `SendAlert` (带声音) | `fs.ErrorPermissionDenied` |
| 423 Locked | `fs.Logf` (NOTICE 级别) | `Send` (无声) | 原 `*api.Error` |
| 507 Insufficient Storage | `fs.Errorf` (ERROR 级别) | `SendAlert` (带声音) | `fs.ErrorQuotaExceeded` |
| 404 Not Found | `fs.Debugf` (DEBUG 级别) | 无 | 原错误（各处已有处理） |
| 其他 | `fs.Debugf` (DEBUG 级别) | 无 | 原错误 |

桌面通知仅在 `f.opt.DesktopNotify == true` 时发送。

#### 3.2.4 修改的操作函数

以下函数增加了 `handleHTTPError` 调用，将 HTTP 错误映射为语义化错误并发送通知：

| 函数 | 操作 | 原有错误处理 | 新增行为 |
|------|------|-------------|----------|
| `readMetaDataForPath` | PROPFIND 读元数据 | 404 → ObjectNotFound | 403 → PermissionDenied + 通知 |
| `listAll` | PROPFIND 列目录 | 404 → DirNotFound | 403 → PermissionDenied + 通知 |
| `_mkdir` | MKCOL 创建目录 | 405/406/423 → 已存在 | 403 → PermissionDenied + 通知 |
| `mkdir` | 递归创建目录 | 409 → 创建父目录 | 403 → PermissionDenied + 通知 |
| `updateSimple` | PUT 上传文件 | 无专门处理 | 403/507 → 语义映射 + 通知 |
| `Remove` | DELETE 删除 | 无专门处理 | 403 → PermissionDenied + 通知 |
| `copyOrMove` | COPY/MOVE 复制移动 | 无专门处理 | 403 → PermissionDenied + 通知 |
| `DirMove` | MOVE 移动目录 | 无专门处理 | 403 → PermissionDenied + 通知 |
| `SetModTime` | PROPPATCH 修改时间 | 404 → ObjectNotFound | 403 → PermissionDenied + 通知 |
| `purgeCheck` | DELETE 删除目录 | 无专门处理 | 403 → PermissionDenied + 通知 |

---

## 4. 用户可见效果

### 4.1 终端输出

```
# 403 权限拒绝
2026/06/12 10:00:00 ERROR : photo.jpg: 权限被拒绝 (403 Forbidden): ...

# 423 文件锁定
2026/06/12 10:00:01 NOTICE : photo.jpg: 文件被锁定 (423 Locked): ...

# 507 空间不足
2026/06/12 10:00:02 ERROR : photo.jpg: 存储空间不足 (507 Insufficient Storage): ...
```

### 4.2 桌面通知

当 `desktop_notify = true`（默认）时：

- **403**: 🔴 "rclone - 权限被拒绝" + 提示音
- **423**: 🔵 "rclone - 文件被锁定" 无声通知
- **507**: 🔴 "rclone - 存储空间不足" + 提示音

---

## 5. 依赖

新增依赖 `github.com/gen2brain/beeep v0.11.2`，及其传递依赖（以下为 `go.mod` 中实际锁定的版本）：

```
github.com/gen2brain/beeep v0.11.2
├── git.sr.ht/~jackmordaunt/go-toast v1.1.2      (Windows Toast 通知)
│   ├── github.com/jackmordaunt/icns/v3 v3.0.1   (macOS ICNS 图标编码)
│   └── github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646  (图标缩放)
├── github.com/esiqveland/notify v0.13.3          (Linux D-Bus 通知)
│   └── github.com/godbus/dbus/v5 v5.1.0          (D-Bus 通信)
├── github.com/tadvi/systray v0.0.0-20190226123456-11a2b8fa57af  (Windows 7 托盘回退)
└── github.com/sergeymakinen/go-ico v1.0.0-beta.0 (ICO 图标编码)
    └── github.com/sergeymakinen/go-bmp v1.0.0    (BMP 格式编码)
```

> **注**：`golang.org/x/sys` 已在 `go.mod` 中存在（`v0.45.0`），并非本次新增。

所有依赖为 **纯 Go**，无 CGO 要求，静态编译进单一二进制文件。

---

## 7. 配置

用户在 `rclone config` 中配置 WebDAV 远端时，可通过 `desktop_notify` 选项控制：

```ini
[mywebdav]
type = webdav
url = https://example.com/webdav
vendor = nextcloud
desktop_notify = true   # 默认为 true，设为 false 关闭桌面通知
```

也可通过命令行覆盖：

```bash
rclone --config rclone.conf copy local: webdav: --webdav-desktop-notify=false
```
