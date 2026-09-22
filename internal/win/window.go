//go:build windows

package win

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

const className = "InbetweenWindow"

var (
	classRegistered bool
	closeRequested  bool
	overlayWindows  = map[uintptr]bool{}
	wndProcCB       = syscall.NewCallback(wndProc)
	pumpMsg         MSG // global, to avoid allocating on every PeekMessage
)

func wndProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch uint32(msg) {
	case WM_CLOSE:
		closeRequested = true
		return 0
	case WM_DESTROY:
		return 0
	case WM_NCHITTEST:
		if overlayWindows[hwnd] {
			return ^uintptr(0) // HTTRANSPARENT
		}
	case WM_MOUSEACTIVATE:
		if overlayWindows[hwnd] {
			return MA_NOACTIVATE
		}
	case WM_KEYDOWN:
		if wp == VK_ESCAPE {
			closeRequested = true
			return 0
		}
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

func registerClass() error {
	if classRegistered {
		return nil
	}
	name, _ := syscall.UTF16PtrFromString(className)
	cursor, _, _ := procLoadCursorW.Call(0, 32512) // IDC_ARROW
	wc := wndClassEx{
		LpfnWndProc:   wndProcCB,
		HInstance:     moduleHandle(),
		HCursor:       cursor,
		LpszClassName: name,
	}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if r == 0 {
		return lastErr("RegisterClassExW", err)
	}
	classRegistered = true
	return nil
}

// Window is the output window: an overlay on top of the game, or a regular window for debugging.
type Window struct {
	HWND    uintptr
	Overlay bool
	Client  RECT // client area in screen coordinates
}

// WindowOptions are the parameters for creating a window.
type WindowOptions struct {
	Title   string
	Client  RECT // desired client area in screen coordinates
	Overlay bool // true: borderless, always-on-top, doesn't steal focus
	Layered bool // for the overlay: WS_EX_LAYERED+WS_EX_TRANSPARENT (clicks pass through to the game)
}

// CreateWindow creates and shows the window (without stealing focus from the game).
func CreateWindow(o WindowOptions) (*Window, error) {
	if err := registerClass(); err != nil {
		return nil, err
	}
	var style, ex uint32
	outer := o.Client
	if o.Overlay {
		style = WS_POPUP
		ex = WS_EX_TOPMOST | WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE
		if o.Layered {
			ex |= WS_EX_LAYERED | WS_EX_TRANSPARENT
		}
	} else {
		style = WS_OVERLAPPEDWINDOW
		outer = AdjustWindowRect(o.Client, style, ex)
	}
	cls, _ := syscall.UTF16PtrFromString(className)
	title, _ := syscall.UTF16PtrFromString(o.Title)
	hwnd, _, err := procCreateWindowExW.Call(
		uintptr(ex), uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title)), uintptr(style),
		uintptr(outer.Left), uintptr(outer.Top), uintptr(outer.W()), uintptr(outer.H()),
		0, 0, moduleHandle(), 0)
	if hwnd == 0 {
		return nil, lastErr("CreateWindowExW", err)
	}
	w := &Window{HWND: hwnd, Overlay: o.Overlay, Client: o.Client}
	if o.Overlay {
		overlayWindows[hwnd] = true
		if o.Layered {
			// A fully opaque layered surface: it exists only so WS_EX_TRANSPARENT works (click-through).
			procSetLayeredWindowAttributes.Call(hwnd, 0, 255, LWA_ALPHA)
		}
		procShowWindow.Call(hwnd, SW_SHOWNOACTIVATE)
		procSetWindowPos.Call(hwnd, hwndTopmost, uintptr(outer.Left), uintptr(outer.Top),
			uintptr(outer.W()), uintptr(outer.H()), SWP_NOACTIVATE|SWP_SHOWWINDOW)
	} else {
		procShowWindow.Call(hwnd, SW_SHOW)
	}
	return w, nil
}

// ExcludeFromCapture excludes the window from screen capture (Windows 10 2004+).
// Critical: without it, Desktop Duplication would capture our own overlay (a feedback loop).
func (w *Window) ExcludeFromCapture() error {
	r, _, err := procSetWindowDisplayAffinity.Call(w.HWND, WDA_EXCLUDEFROMCAPTURE)
	if r == 0 {
		return lastErr("SetWindowDisplayAffinity(WDA_EXCLUDEFROMCAPTURE)", err)
	}
	return nil
}

func (w *Window) SetVisible(v bool) {
	if v {
		procShowWindow.Call(w.HWND, SW_SHOWNOACTIVATE)
		if w.Overlay {
			procSetWindowPos.Call(w.HWND, hwndTopmost, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)
		}
	} else {
		procShowWindow.Call(w.HWND, SW_HIDE)
	}
}

func (w *Window) Destroy() {
	if w.HWND != 0 {
		delete(overlayWindows, w.HWND)
		procDestroyWindow.Call(w.HWND)
		w.HWND = 0
	}
}

// PumpMessages processes all queued thread messages without blocking.
// Returns true if the user asked to quit (closed the window, Esc, WM_QUIT).
func PumpMessages(onHotkey func(id int)) (quit bool) {
	for {
		r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&pumpMsg)), 0, 0, 0, PM_REMOVE)
		if r == 0 {
			break
		}
		switch pumpMsg.Message {
		case WM_QUIT:
			quit = true
			continue
		case WM_HOTKEY:
			if onHotkey != nil {
				onHotkey(int(pumpMsg.WParam))
			}
			continue
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&pumpMsg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&pumpMsg)))
	}
	return quit || closeRequested
}

// WindowInfo describes a visible top-level window.
type WindowInfo struct {
	HWND   uintptr
	Title  string
	PID    uint32
	Client RECT // client area in screen coordinates
}

func (w WindowInfo) String() string {
	return fmt.Sprintf("hwnd=0x%X pid=%d client=%v %q", w.HWND, w.PID, w.Client, w.Title)
}

var (
	enumResult []uintptr
	enumCB     = syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		enumResult = append(enumResult, hwnd)
		return 1
	})
)

func windowTitle(hwnd uintptr) string {
	n, _, _ := procGetWindowTextLengthW.Call(hwnd)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}

func isCloaked(hwnd uintptr) bool {
	if procDwmGetWindowAttribute.Find() != nil {
		return false
	}
	var cloaked uint32
	r, _, _ := procDwmGetWindowAttribute.Call(hwnd, dwmwaCloaked, uintptr(unsafe.Pointer(&cloaked)), 4)
	return r == 0 && cloaked != 0
}

// ListWindows returns visible windows that have a title and a non-empty client area
// (excluding our own).
func ListWindows() []WindowInfo {
	enumResult = enumResult[:0]
	procEnumWindows.Call(enumCB, 0)
	self := uint32(os.Getpid())
	var res []WindowInfo
	for _, h := range enumResult {
		if v, _, _ := procIsWindowVisible.Call(h); v == 0 || IsIconic(h) || isCloaked(h) {
			continue
		}
		title := windowTitle(h)
		if title == "" {
			continue
		}
		var pid uint32
		procGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&pid)))
		if pid == self {
			continue
		}
		rc, err := ClientRectOnScreen(h)
		if err != nil || rc.Empty() {
			continue
		}
		res = append(res, WindowInfo{HWND: h, Title: title, PID: pid, Client: rc})
	}
	return res
}

// FindWindow looks for visible windows whose title contains substr (case-insensitive).
// Returns the first match and the list of all matches.
func FindWindow(substr string) (WindowInfo, []WindowInfo, error) {
	s := strings.ToLower(substr)
	var matches []WindowInfo
	for _, w := range ListWindows() {
		if strings.Contains(strings.ToLower(w.Title), s) {
			matches = append(matches, w)
		}
	}
	if len(matches) == 0 {
		return WindowInfo{}, nil, fmt.Errorf("окно с %q в заголовке не найдено (см. -mode list)", substr)
	}
	return matches[0], matches, nil
}
