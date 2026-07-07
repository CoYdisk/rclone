// Package notify provides cross-platform desktop notification capabilities.
package notify

import (
	"github.com/gen2brain/beeep"
	"github.com/rclone/rclone/fs"
)

// Send sends a normal desktop notification without sound.
// Failures are silently ignored (DEBUG logged).
func Send(title, message string) {
	err := beeep.Notify(title, message, "")
	if err != nil {
		fs.Debugf(nil, "notify: failed to send notification: %v", err)
	}
}

// SendAlert sends an urgent desktop notification with sound.
// Failures are silently ignored (DEBUG logged).
func SendAlert(title, message string) {
	err := beeep.Alert(title, message, "")
	if err != nil {
		fs.Debugf(nil, "notify: failed to send alert notification: %v", err)
	}
}
