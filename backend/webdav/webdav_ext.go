// 🏢 悦库修改 — 自动卸载挂载点功能
// WebDAV 后端暴露 pacer 给 mount 层，用于监控重试耗尽时自动卸载。
package webdav

import "github.com/rclone/rclone/fs"

// GetPacer returns the pacer used by this Fs for API calls.
// This allows the mount layer to monitor when all retries are exhausted
// and trigger an automatic unmount if the remote becomes unreachable.
func (f *Fs) GetPacer() *fs.Pacer {
	return f.pacer
}
