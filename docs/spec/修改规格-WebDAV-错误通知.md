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
- 跨平台：Windows Toast/托盘气泡、Linux D-Bus/notify-send、macOS terminal-notifier → osascript 回退
- 使用 beeep 库实现，无 CGO 依赖
- `init()` 中设置 `beeep.AppName = "网盘"`，通知来源显示为 "网盘"（影响 Linux D-Bus/notify-send、macOS terminal-notifier、Windows Toast 的各平台 AppID/分组名）
- 内嵌 rclone logo（`graphics/logo/favicon/favicon_32px.png`，base64 编码，解码后传给 beeep 作为通知图标），解码失败时回退空字符串
- 通知文本使用中文（标题如 "网盘 - 权限被拒绝"，消息体中的路径和错误详情经过 `sanitizeNotifyMsg` 清洗后再展示）

**实现细节**:
```go
func init() {
    beeep.AppName = "网盘"
    rcloneIcon, _ = base64.StdEncoding.DecodeString(rcloneIconBase64)
}
```

- `rcloneIconBase64` 常量包含 base64 编码的 32x32 PNG 图标
- `rcloneIcon` 是解码后的 `[]byte`，传给 `beeep.Notify/Alert` 的 icon 参数
- 各平台图标处理：
  - macOS terminal-notifier：PNG → ICNS 转换后显示
  - macOS osascript：不使用图标（通知正常显示）
  - Windows Toast：直接使用 PNG 路径
  - Windows 7 balloon：PNG → ICO 转换后显示
  - Linux D-Bus：解码为 RGBA 图像数据
  - Linux notify-send：使用 PNG 文件路径

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

**消息提取与清洗逻辑**:

1. 优先使用 `apiErr.Message`（服务端返回的 `<s:message>` XML 元素）
2. 当 `apiErr.Message` 为空时，使用 `apiErr.Error()` — 该函数依次拼接 `Message` → `Exception` → `Status`，若全部为空则返回 `"Webdav Error"`
3. 终端日志使用原始 `detailMsg`（保留完整调试信息）
4. 桌面通知使用 `sanitizeNotifyMsg(remote + ": " + detailMsg)` 清洗后的文本（详见 §3.2.6）

这样确保：
- 终端日志中始终有完整的服务器返回信息，便于故障排查
- 桌面通知中的路径和错误消息干净可读，不会出现 XML 标签或 URL 编码乱码

#### 3.2.4 修改方法: `errorHandler`

**位置**: `NewFs` 中通过 `f.srv.SetErrorHandler(errorHandler)` 注册。

**原有行为**: 解析服务端错误响应，仅当 XML 解析失败时将原始 body 作为 `Message`。

**新增行为**: 即使 XML 解析成功，如果 `Message` 字段仍为空（服务端使用了非标准的错误 XML 格式，如 `<D:responsedescription>` 或无 namespace 前缀的 `<message>`），也回退使用原始响应 body 作为 `Message`。

```go
// 新增回退逻辑：XML 解析成功但 Message 为空时，使用原始 body
if errResponse.Message == "" {
    errResponse.Message = strings.TrimSpace(string(body))
}
```

这样无论 WebDAV 服务端使用何种错误响应格式（SabreDAV 的 `<s:message>`、纯文本、HTML、非标准 XML），`apiErr.Message` 始终包含服务端返回的信息。

#### 3.2.5 修改的操作函数

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

#### 3.2.6 新增方法: `sanitizeNotifyMsg`

```go
func sanitizeNotifyMsg(raw string) string
```

**用途**: 清洗服务端错误消息和文件路径，使其适合在桌面气泡通知中显示。

**位置**: `handleHTTPError` 之前，作为包级函数。

**处理流程**（按顺序执行）:

| 步骤 | 操作 | 说明 |
|------|------|------|
| 1. 去除 XML/HTML 标签 | 正则 `<[^>]*>` 匹配并移除 | 服务端错误响应体常包含 `<D:error>`、`<s:message>` 等 XML 标签 |
| 2. 压缩空白字符 | `strings.Fields` + `strings.Join` | 换行、制表符、连续空格 → 单个空格 |
| 3. URL 解码 | 优先 `url.PathUnescape`，回退 `url.QueryUnescape`，循环最多 3 次 | 处理 `%E5%8D%8F` → `协`、双重编码 `%25E5%258D%258F` 等情况 |
| 4. 截断超长内容 | 超过 256 字符截断并追加 `...` | Windows 气泡通知最大约 256 字符 |
| 5. 空内容回退 | 清洗后为空则返回 `"(无详细信息)"` | 确保通知始终有内容可显示 |

**设计要点**:

- **`url.PathUnescape` 优先**：不会将 `+` 误当作空格（WebDAV 文件路径中 `+` 是合法字符），比 `QueryUnescape` 更适合作路径解码
- **双重编码处理**：某些代理/中间件会对 URL 做二次编码（`%` → `%25`），循环解码确保还原原始文本
- **仅用于通知**：终端日志仍使用原始 `detailMsg`，保留完整调试信息

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

- **403**: 🔴 "网盘 - 权限被拒绝" + 提示音
- **423**: 🔵 "网盘 - 文件被锁定" 无声通知
- **507**: 🔴 "网盘 - 存储空间不足" + 提示音

通知消息体格式为 `文件路径: 错误详情`，其中路径和详情均经过 `sanitizeNotifyMsg` 清洗（去除 XML 标签、URL 解码、截断），确保在 Windows 气泡/Linux 通知/macOS 通知中正常显示中文。

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

## 6. 配置

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
