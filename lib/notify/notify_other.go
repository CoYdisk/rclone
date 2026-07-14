//go:build !darwin

// Package notify provides a no-op stub for sendDarwinNotification on non-macOS platforms.
package notify

// sendDarwinNotification is a no-op on non-macOS platforms.
// The actual macOS implementations are in notify_darwin_cgo.go and notify_darwin_no_cgo.go.
func sendDarwinNotification(title, message string, alert bool) {
	// Not supported on this platform
}
