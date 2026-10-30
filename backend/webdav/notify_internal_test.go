// 桌面通知的回归用例（package webdav 内部测试：handleHTTPError 未导出）。
//
// 为什么必须有这一条：`backend/webdav` 的通知分支写在 handleHTTPError 里，而
// **可重试**的状态码（423 Locked / 500 / 507 等）在到达这里之前已经被
// `fs/pacer.go:pacerInvoker` 用 `fserrors.RetryError()` 包了一层；
// 只有"不重试"的状态码（403）才是裸 `*api.Error`。
// 于是早先那版 `err.(*api.Error)` 直接断言对重试类错误恒为 false ⇒
// **「文件被锁定」的通知永远不会发**，而 403 一切正常（真机 A/B 就是这样：
// 403 的横幅在 VM 上弹得出来，423 连日志行都没有）。
package webdav

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/rclone/rclone/backend/webdav/api"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/fserrors"
)

// stubNotify 把包内两条通知出口换成记录器
func stubNotify(records *[]string) (restore func()) {
	oldSend, oldAlert := notifySend, notifySendAlert
	notifySend = func(title, message string) { *records = append(*records, "send|"+title+"|"+message) }
	notifySendAlert = func(title, message string) { *records = append(*records, "alert|"+title+"|"+message) }
	return func() { notifySend, notifySendAlert = oldSend, oldAlert }
}

// TestHandleHTTPErrorNotifiesRetryableStatuses 钉住"可重试错误也必须发通知"。
// 每条都按**包装后**的错误投喂（与真机路径一致），否则只能覆盖 403。
func TestHandleHTTPErrorNotifiesRetryableStatuses(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		want   string // 期望的通知标题
		alert  bool
	}{
		{"403 权限被拒绝", http.StatusForbidden, "悦库企业网盘 - 权限被拒绝", true},
		{"423 文件被锁定", http.StatusLocked, "悦库企业网盘 - 文件被锁定", false},
		{"500 服务器内部错误", http.StatusInternalServerError, "悦库企业网盘 - 服务器内部错误", true},
		{"507 存储空间不足", http.StatusInsufficientStorage, "悦库企业网盘 - 存储空间不足", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, wrapped := range []bool{false, true} {
				tag := fmt.Sprintf("wrapped=%v", wrapped)
				var records []string
				defer stubNotify(&records)()
				f := &Fs{opt: Options{DesktopNotify: true}}

				var err error = &api.Error{StatusCode: test.status, Message: "服务端说明"}
				if wrapped {
					// 与 fs/pacer.go:pacerInvoker 对可重试错误的处理一致
					err = fserrors.RetryError(err)
				}
				_ = f.handleHTTPError(context.Background(), err, "协同空间/test/a.txt", "updateSimple")

				if len(records) != 1 {
					t.Fatalf("%s %s: 期望正好 1 条通知，实际 %d 条 %v", test.name, tag, len(records), records)
				}
				kind := "send"
				if test.alert {
					kind = "alert"
				}
				wantPrefix := kind + "|" + test.want + "|"
				if len(records[0]) < len(wantPrefix) || records[0][:len(wantPrefix)] != wantPrefix {
					t.Fatalf("%s %s: 通知不符，期望前缀 %q，实际 %q", test.name, tag, wantPrefix, records[0])
				}
			}
		})
	}
}

// TestHandleHTTPErrorNotifiesDisabled 关掉开关后一条都不许发（真机排查时才不会被噪音误导）
func TestHandleHTTPErrorNotifiesDisabled(t *testing.T) {
	var records []string
	defer stubNotify(&records)()
	f := &Fs{opt: Options{DesktopNotify: false}}
	_ = f.handleHTTPError(context.Background(), fserrors.RetryError(&api.Error{StatusCode: http.StatusLocked}), "a/b.txt", "updateSimple")
	if len(records) != 0 {
		t.Fatalf("DesktopNotify=false 时不该发通知，实际 %v", records)
	}
}

// TestHandleHTTPErrorIgnoresNonHTTPError 非 HTTP 错误不该被误当成 423/500 发通知
func TestHandleHTTPErrorIgnoresNonHTTPError(t *testing.T) {
	var records []string
	defer stubNotify(&records)()
	f := &Fs{opt: Options{DesktopNotify: true}}
	orig := errors.New("connection reset by peer")
	_ = f.handleHTTPError(context.Background(), orig, "a/b.txt", "updateSimple")
	if len(records) != 0 {
		t.Fatalf("非 HTTP 错误不该发通知，实际 %v", records)
	}
}

// TestHandleHTTPErrorReturnsSentinelForMappedCodes 返回值语义不变（403/507 映射成哨兵错误）
func TestHandleHTTPErrorReturnsSentinelForMappedCodes(t *testing.T) {
	var records []string
	defer stubNotify(&records)()
	f := &Fs{opt: Options{DesktopNotify: false}}
	if err := f.handleHTTPError(context.Background(), fserrors.RetryError(&api.Error{StatusCode: http.StatusForbidden}), "a/b.txt", "updateSimple"); !errors.Is(err, fs.ErrorPermissionDenied) && err != fs.ErrorPermissionDenied {
		t.Fatalf("403 应映射成 fs.ErrorPermissionDenied，实际 %v", err)
	}
	if err := f.handleHTTPError(context.Background(), fserrors.RetryError(&api.Error{StatusCode: http.StatusInsufficientStorage}), "a/b.txt", "updateSimple"); err != fs.ErrorQuotaExceeded {
		t.Fatalf("507 应映射成 fs.ErrorQuotaExceeded，实际 %v", err)
	}
}
