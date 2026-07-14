//go:build darwin && cgo

// Package notify provides macOS-specific notification using native APIs.
// On macOS, osascript "display notification" may not produce visible
// notifications from unsigned command-line tools due to macOS code
// signing and entitlement requirements. This file uses the deprecated
// but functional NSUserNotificationCenter API via cgo, which works
// without special entitlements for basic notifications.
package notify

/*
#cgo CFLAGS: -x objective-c -Wno-deprecated-declarations
#cgo LDFLAGS: -framework Foundation

#import <Foundation/Foundation.h>
#import <stdlib.h>

// sendMacNotification posts a user notification using the deprecated
// NSUserNotificationCenter API. This API still works on all macOS versions
// and does not require code signing or special entitlements for basic use.
static void sendMacNotification(const char *title, const char *message, int alert) {
	@autoreleasepool {
		NSUserNotificationCenter *center = [NSUserNotificationCenter defaultUserNotificationCenter];
		if (!center) {
			return;
		}

		NSUserNotification *notification = [[NSUserNotification alloc] init];
		if (!notification) {
			return;
		}

		// Set title
		NSString *nsTitle = [NSString stringWithUTF8String:title];
		if (nsTitle) {
			[notification setTitle:nsTitle];
		}

		// Set informative text (body)
		NSString *nsMessage = [NSString stringWithUTF8String:message];
		if (nsMessage) {
			[notification setInformativeText:nsMessage];
		}

		// Set sound if alert
		if (alert) {
			NSString *defaultSound = [NSUserNotification defaultSoundName];
			if (defaultSound) {
				[notification setSoundName:defaultSound];
			}
		}

		// Deliver the notification
		[center deliverNotification:notification];

		// Pump the run loop to allow the notification to be delivered.
		// NSUserNotificationCenter schedules delivery on the run loop.
		[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.5]];
	}
}
*/
import "C"
import (
	"unsafe"
)

// sendDarwinNotification sends a desktop notification on macOS using the native
// NSUserNotificationCenter API. This is more reliable than osascript because it
// uses the Objective-C notification system directly.
//
// The notification is sent asynchronously in a goroutine to avoid blocking
// the caller (e.g., a FUSE mount callback).
func sendDarwinNotification(title, message string, alert bool) {
	// Copy strings for the goroutine
	t := title
	m := message

	go func() {
		cTitle := C.CString(t)
		cMessage := C.CString(m)
		defer C.free(unsafe.Pointer(cTitle))
		defer C.free(unsafe.Pointer(cMessage))

		alertInt := 0
		if alert {
			alertInt = 1
		}

		C.sendMacNotification(cTitle, cMessage, C.int(alertInt))
	}()
}
