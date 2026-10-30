// 通知去重的回归用例（package notify 内部测试：shouldNotify 未导出）。
//
// 为什么必须有：一次失败的上传在 rclone 侧会多次进入 handleHTTPError（低层重试 +
// updateSimple 失败后那次 Remove），真机实测**一次写入弹出 6 条一模一样的横幅**。
// 去重窗口内同一条只留一条，但"换了正文/换了标题"仍然要能弹出来。
package notify

import (
	"testing"
	"time"
)

func TestShouldNotifyDedupesIdenticalWithinWindow(t *testing.T) {
	dedupeMu.Lock()
	dedupeLast = map[string]time.Time{}
	dedupeMu.Unlock()

	now := time.Now()
	const title = "悦库企业网盘 - 文件被锁定"
	const body = "协同空间/test/a.txt: 文件正在被其他用户使用 (423 Locked)"

	if !shouldNotify(title, body, now) {
		t.Fatal("第一次应当放行")
	}
	if shouldNotify(title, body, now.Add(time.Second)) {
		t.Fatal("窗口内的重复通知应被抑制")
	}
	if shouldNotify(title, body, now.Add(dedupeWindow-time.Second)) {
		t.Fatal("窗口边界内仍应被抑制")
	}
	if !shouldNotify(title, body, now.Add(dedupeWindow+time.Second)) {
		t.Fatal("超过窗口后应再次放行")
	}
}

func TestShouldNotifyKeepsDistinctMessages(t *testing.T) {
	dedupeMu.Lock()
	dedupeLast = map[string]time.Time{}
	dedupeMu.Unlock()

	now := time.Now()
	const title = "悦库企业网盘 - 文件被锁定"
	if !shouldNotify(title, "a.txt: 423", now) {
		t.Fatal("a.txt 第一次应放行")
	}
	if !shouldNotify(title, "b.txt: 423", now) {
		t.Fatal("不同文件（正文不同）应各自放行")
	}
	if !shouldNotify("悦库企业网盘 - 权限被拒绝", "a.txt: 403", now) {
		t.Fatal("不同标题应各自放行")
	}
}

func TestShouldNotifyPrunesExpiredKeys(t *testing.T) {
	dedupeMu.Lock()
	dedupeLast = map[string]time.Time{}
	dedupeMu.Unlock()

	now := time.Now()
	for i := 0; i < 50; i++ {
		shouldNotify("t", string(rune('a'+i%26)), now.Add(-2*dedupeWindow))
	}
	shouldNotify("t", "fresh", now)

	dedupeMu.Lock()
	size := len(dedupeLast)
	dedupeMu.Unlock()
	if size != 1 {
		t.Fatalf("过期键应被清理，期望只剩 1 条，实际 %d 条", size)
	}
}
