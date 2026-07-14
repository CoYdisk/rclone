//go:build darwin && !cgo

// Package notify provides macOS-specific notification using osascript.
// This is the fallback for builds without cgo (though cmount builds
// always have cgo enabled on macOS).
package notify

import (
	"fmt"
	"os/exec"
)

// sendDarwinNotification sends a desktop notification on macOS using osascript.
func sendDarwinNotification(title, message string, alert bool) {
	err := sendOSAScript(title, message, alert)
	if err != nil {
		return
	}
	// Silently ignore - error logged by caller
	_ = err
}

// sendOSAScript sends a desktop notification on macOS using osascript.
func sendOSAScript(title, message string, alert bool) error {
	osa, err := exec.LookPath("osascript")
	if err != nil {
		return fmt.Errorf("osascript not found: %w", err)
	}

	escTitle := escapeAppleScript(title)
	escMessage := escapeAppleScript(message)

	var script string
	if alert {
		script = fmt.Sprintf(`display notification "%s" with title "%s" sound name "default"`, escMessage, escTitle)
	} else {
		script = fmt.Sprintf(`display notification "%s" with title "%s"`, escMessage, escTitle)
	}

	cmd := exec.Command(osa, "-e", script)
	return cmd.Run()
}

// escapeAppleScript escapes a string for use inside an AppleScript double-quoted string literal.
func escapeAppleScript(s string) string {
	result := make([]byte, 0, len(s)+10)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			result = append(result, '\\', '\\')
		case '"':
			result = append(result, '\\', '"')
		case '\n':
			result = append(result, '\\', 'n')
		case '\r':
			result = append(result, '\\', 'r')
		case '\t':
			result = append(result, '\\', 't')
		default:
			result = append(result, c)
		}
	}
	return string(result)
}
