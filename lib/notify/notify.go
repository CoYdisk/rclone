// Package notify provides cross-platform desktop notification capabilities.
package notify

import (
	"encoding/base64"

	"github.com/gen2brain/beeep"
	"github.com/rclone/rclone/fs"
)

func init() {
	beeep.AppName = "网盘"

	var err error
	rcloneIcon, err = base64.StdEncoding.DecodeString(rcloneIconBase64)
	if err != nil {
		fs.Debugf(nil, "notify: failed to decode embedded icon: %v", err)
	}
}

// rcloneIconBase64 is the embedded rclone logo (32x32 PNG) as base64.
// It is decoded into rcloneIcon at init time for use as the notification icon.
const rcloneIconBase64 = "iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAYAAABzenr0AAACh0lEQVR4AWKgBtjy+CcjA6DWegCSJIuiMLy2bXvHtm3btm3btm3btm2bbd/5c1jx4iW7piO+Lt9z0un/P30Q8mE1TuIWInELI5EFr7+s8CI4ALFxEzn8Hd4c4kIkKvsrvAxiIB60i214ekRAPIpBnNgU2AwxsRrt0QIbEA3RmO41/F+IRhiya76fF8EQRTT+9lKgE0SjqsVvqkI0WngpMAGiiMLHFr95C3chit5eCiyG4A42YSmGOfjdIYhirJcC8fFN+pZzf0ZBVPJRyOJMeRmimOsqnIA8WIHbEBPpNAVSQzTaNNgV8LuT4I8xAeLAfrymFFgL0fiTAoewDN+Zhf+PKxAX/vcJLwTR2E9oQshTd/CXGv4m9kNcmOUT/jbOQTQaEjgA4uMYPvQt0BHiQgh+9inwPUIhirPt9wf+RFgARDHpWfg/iIRoHENFxMNPPr7S7ICdIYqCBHWHaIThI6NAHYjGIrzj4rB9D0dx+6nFxvuEfIKdEI2yr5js9ffxpb/uJ56WCIEoFhsFjkAUCy2GvYV9uP3UOrzqoMQciOKqUSAAohhkMagZRFHeQYFREMURo8BWiGKDyZDPEQBRXMMHNgVWQxSbjAIDIYpIJNYMeQNHIBpWay0eYiCK+UaBchCNU/hfMywzxEQTvKp8/19cgGiUMQp8jlsQjTBMR1MMx1lkNJpDTBxBZzTBPIRCNE7j9Wcno8IQhw5VX331t6fbXWKhkno9mApxaCwDEiAA4sECvKEW+ATzIA41ZkhWREBcGILXrO4JSuEuxMYufMCw5FgOsRGCJk7viL5BI0zDScQgEicxDzk0R0cSzMJRXEMYQjAHxfG+Ps1ZoffxhodrwGt233kE0TDRwOgLmIgAAAAASUVORK5CYII="

// rcloneIcon holds the decoded rclone logo PNG bytes for use as notification icon.
// nil if decoding failed; Send/SendAlert will fall back to an empty string icon.
var rcloneIcon []byte

// Send sends a normal desktop notification without sound.
// Failures are silently ignored (DEBUG logged).
func Send(title, message string) {
	var icon any = ""
	if rcloneIcon != nil {
		icon = rcloneIcon
	}
	err := beeep.Notify(title, message, icon)
	if err != nil {
		fs.Debugf(nil, "notify: failed to send notification: %v", err)
	}
}

// SendAlert sends an urgent desktop notification with sound.
// Failures are silently ignored (DEBUG logged).
func SendAlert(title, message string) {
	var icon any = ""
	if rcloneIcon != nil {
		icon = rcloneIcon
	}
	err := beeep.Alert(title, message, icon)
	if err != nil {
		fs.Debugf(nil, "notify: failed to send alert notification: %v", err)
	}
}
