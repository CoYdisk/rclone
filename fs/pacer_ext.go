// 🏢 悦库修改 — 自动卸载挂载点功能
// 透传 lib/pacer 的 SetOnMaxRetries 到 fs.Pacer 层。
package fs

// SetOnMaxRetries sets a callback on the underlying pacer that is
// called when all retries are exhausted. The last error from the
// retry loop is passed to it.
func (p *Pacer) SetOnMaxRetries(f func(err error)) {
	p.Pacer.SetOnMaxRetries(f)
}
