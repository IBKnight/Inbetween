//go:build windows

package gfx

import (
	"strings"
	"unsafe"
)

var severityNames = [...]string{"CORRUPTION", "ERROR", "WARNING", "INFO", "MESSAGE"}

// DrainDebugMessages flushes the D3D11 debug layer's queued messages (only works with -debug).
func (d *Device) DrainDebugMessages(logf func(format string, args ...any)) {
	if d.info.Nil() {
		return
	}
	n := int(d.info.Call(infoGetNumStoredMessages))
	for i := 0; i < n; i++ {
		var size uintptr
		d.info.Call(infoGetMessage, uintptr(i), 0, uintptr(unsafe.Pointer(&size)))
		if size == 0 {
			continue
		}
		buf := make([]uint64, (size+7)/8)
		d.info.Call(infoGetMessage, uintptr(i), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
		msg := (*d3d11Message)(unsafe.Pointer(&buf[0]))
		if msg.PDescription == nil || msg.DescriptionByteLength == 0 {
			continue
		}
		text := strings.TrimRight(unsafe.String(msg.PDescription, int(msg.DescriptionByteLength)), "\x00")
		sev := "?"
		if msg.Severity >= 0 && int(msg.Severity) < len(severityNames) {
			sev = severityNames[msg.Severity]
		}
		logf("[d3d11 %s] %s", sev, text)
	}
	d.info.Call(infoClearStoredMessages)
}
