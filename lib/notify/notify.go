// Package notify provides cross-platform desktop notification capabilities.
package notify

import (
	"encoding/base64"
	"runtime"
	"sync"
	"time"

	"github.com/gen2brain/beeep"
	"github.com/rclone/rclone/fs"
)

func init() {
	beeep.AppName = "悦库企业网盘"

	var err error
	rcloneIcon, err = base64.StdEncoding.DecodeString(rcloneIconBase64)
	if err != nil {
		// 在 ERROR 级留一条：图标解不出来时通知只剩应用名，静默降级会让人以为"图标改了没生效"。
		fs.Errorf(nil, "notify: failed to decode embedded icon: %v", err)
	}
}

// rcloneIconBase64 是悦库蓝底「y」图标（PNG，来自 client/electron/assets/ydisk.ico）的 base64。
// It is decoded into rcloneIcon at init time for use as the notification icon.
const rcloneIconBase64 = "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"

// rcloneIcon holds the decoded rclone logo PNG bytes for use as notification icon.
// nil if decoding failed; Send/SendAlert will fall back to an empty string icon.
var rcloneIcon []byte

// dedupeWindow is how long an identical notification (same title+message) is
// suppressed after it has been shown once.
//
// 为什么需要：一次失败的上传在 rclone 侧会走多次尝试 —— 低层重试（423/500/507 都在
// retryErrorCodes 里）以及 updateSimple 失败后对目标的那次 `Remove`，每次都会进
// handleHTTPError ⇒ 同一条「文件被锁定」会连弹好几个横幅（真机实测一次写入 6 条）。
// 用户看到的应当是"这件事出错了"，而不是"出错了 6 次"。
const dedupeWindow = 60 * time.Second

var (
	dedupeMu   sync.Mutex
	dedupeLast = map[string]time.Time{}
)

// shouldNotify reports whether this notification should be shown, and records it.
// 纯内部状态，供 Send/SendAlert 调用；单测直接调它（不会真弹通知）。
func shouldNotify(title, message string, now time.Time) bool {
	key := title + "\x00" + message
	dedupeMu.Lock()
	defer dedupeMu.Unlock()
	// 顺手清掉过期的键，避免长时间运行时 map 无限增长
	for k, t := range dedupeLast {
		if now.Sub(t) >= dedupeWindow {
			delete(dedupeLast, k)
		}
	}
	if last, ok := dedupeLast[key]; ok && now.Sub(last) < dedupeWindow {
		return false
	}
	dedupeLast[key] = now
	return true
}

// Send sends a normal desktop notification without sound.
// Failures are logged at ERROR level so they are visible with --verbose.
func Send(title, message string) {
	if !shouldNotify(title, message, time.Now()) {
		fs.Debugf(nil, "notify: suppressed duplicate notification %q (within %v)", title, dedupeWindow)
		return
	}
	if runtime.GOOS == "darwin" {
		sendDarwinNotification(title, message, false)
		return
	}

	var icon any = ""
	if rcloneIcon != nil {
		icon = rcloneIcon
	}
	err := beeep.Notify(title, message, icon)
	if err != nil {
		fs.Errorf(nil, "notify: failed to send notification: %v", err)
	}
}

// SendAlert sends an urgent desktop notification with sound.
// Failures are logged at ERROR level so they are visible with --verbose.
func SendAlert(title, message string) {
	if !shouldNotify(title, message, time.Now()) {
		fs.Debugf(nil, "notify: suppressed duplicate alert %q (within %v)", title, dedupeWindow)
		return
	}
	if runtime.GOOS == "darwin" {
		sendDarwinNotification(title, message, true)
		return
	}

	var icon any = ""
	if rcloneIcon != nil {
		icon = rcloneIcon
	}
	err := beeep.Alert(title, message, icon)
	if err != nil {
		fs.Errorf(nil, "notify: failed to send alert notification: %v", err)
	}
}
