// 🏢 悦库修改 — 自动卸载挂载点功能
// 挂载层统一入口：检测后端是否实现了 PacerProvider 接口，
// 若是则注册重试耗尽回调，自动触发卸载。
package mountlib

import (
	"sync"

	"github.com/rclone/rclone/fs"
)

// PacerProvider is an interface that backends can optionally implement
// to expose their pacer for connection failure monitoring.
type PacerProvider interface {
	GetPacer() *fs.Pacer
}

// AutoUnmountOnFailure sets up monitoring on the backend's pacer so that
// when all API retries are exhausted, the unmount function is called.
// This prevents the mounted drive from becoming stuck/unusable when the
// remote becomes unreachable (e.g. WebDAV server down).
func AutoUnmountOnFailure(f fs.Fs, unmountFn func() error) {
	pacerProv, ok := f.(PacerProvider)
	if !ok {
		fs.Debugf(f, "Auto-unmount on failure not supported by this backend")
		return
	}
	p := pacerProv.GetPacer()
	if p == nil {
		return
	}
	var once sync.Once
	fs.Debugf(f, "Auto-unmount on connection failure enabled")
	p.SetOnMaxRetries(func(err error) {
		once.Do(func() {
			fs.Errorf(f, "Remote unreachable after all retries, unmounting: %v", err)
			if unmountErr := unmountFn(); unmountErr != nil {
				fs.Errorf(f, "Failed to auto-unmount: %v", unmountErr)
			}
		})
	})
}
