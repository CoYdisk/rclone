// 🏢 悦库修改 — 自动卸载挂载点功能
// 当后端 API 重试全部耗尽时触发卸载，防止远程不可达导致虚拟盘卡死。
package pacer

// OnMaxRetriesOption sets a callback that is called when all retries
// are exhausted. The last error from the retry loop is passed to it.
func OnMaxRetriesOption(f func(err error)) Option {
	return func(p *pacerOptions) { p.onMaxRetries = f }
}

// SetOnMaxRetries sets a callback that is called when all retries
// are exhausted. The last error from the retry loop is passed to it.
func (p *Pacer) SetOnMaxRetries(f func(err error)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onMaxRetries = f
}
