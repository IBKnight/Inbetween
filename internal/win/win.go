//go:build windows

package win

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	winmm    = syscall.NewLazyDLL("winmm.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")

	procRegisterClassExW              = user32.NewProc("RegisterClassExW")
	procCreateWindowExW               = user32.NewProc("CreateWindowExW")
	procDefWindowProcW                = user32.NewProc("DefWindowProcW")
	procDestroyWindow                 = user32.NewProc("DestroyWindow")
	procShowWindow                    = user32.NewProc("ShowWindow")
	procPeekMessageW                  = user32.NewProc("PeekMessageW")
	procTranslateMessage              = user32.NewProc("TranslateMessage")
	procDispatchMessageW              = user32.NewProc("DispatchMessageW")
	procSetWindowPos                  = user32.NewProc("SetWindowPos")
	procSetLayeredWindowAttributes    = user32.NewProc("SetLayeredWindowAttributes")
	procSetWindowDisplayAffinity      = user32.NewProc("SetWindowDisplayAffinity")
	procEnumWindows                   = user32.NewProc("EnumWindows")
	procGetWindowTextW                = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW          = user32.NewProc("GetWindowTextLengthW")
	procIsWindowVisible               = user32.NewProc("IsWindowVisible")
	procIsWindow                      = user32.NewProc("IsWindow")
	procIsIconic                      = user32.NewProc("IsIconic")
	procGetClientRect                 = user32.NewProc("GetClientRect")
	procClientToScreen                = user32.NewProc("ClientToScreen")
	procGetWindowThreadProcessId      = user32.NewProc("GetWindowThreadProcessId")
	procRegisterHotKey                = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey              = user32.NewProc("UnregisterHotKey")
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procLoadCursorW                   = user32.NewProc("LoadCursorW")
	procAdjustWindowRectEx            = user32.NewProc("AdjustWindowRectEx")

	procQueryPerformanceCounter   = kernel32.NewProc("QueryPerformanceCounter")
	procQueryPerformanceFrequency = kernel32.NewProc("QueryPerformanceFrequency")
	procCreateWaitableTimerExW    = kernel32.NewProc("CreateWaitableTimerExW")
	procSetWaitableTimer          = kernel32.NewProc("SetWaitableTimer")
	procWaitForSingleObject       = kernel32.NewProc("WaitForSingleObject")
	procCloseHandle               = kernel32.NewProc("CloseHandle")
	procGetModuleHandleW          = kernel32.NewProc("GetModuleHandleW")
	procGetCurrentThread          = kernel32.NewProc("GetCurrentThread")
	procSetThreadPriority         = kernel32.NewProc("SetThreadPriority")

	procTimeBeginPeriod = winmm.NewProc("timeBeginPeriod")
	procTimeEndPeriod   = winmm.NewProc("timeEndPeriod")

	procDwmGetWindowAttribute = dwmapi.NewProc("DwmGetWindowAttribute")
)

const (
	WS_POPUP            = 0x80000000
	WS_OVERLAPPEDWINDOW = 0x00CF0000

	WS_EX_TOPMOST     = 0x00000008
	WS_EX_TRANSPARENT = 0x00000020
	WS_EX_TOOLWINDOW  = 0x00000080
	WS_EX_LAYERED     = 0x00080000
	WS_EX_NOACTIVATE  = 0x08000000

	LWA_ALPHA = 0x2

	SW_HIDE           = 0
	SW_SHOWNOACTIVATE = 4
	SW_SHOW           = 5

	SWP_NOSIZE     = 0x0001
	SWP_NOMOVE     = 0x0002
	SWP_NOACTIVATE = 0x0010
	SWP_SHOWWINDOW = 0x0040

	PM_REMOVE = 0x1

	WM_DESTROY       = 0x0002
	WM_CLOSE         = 0x0010
	WM_QUIT          = 0x0012
	WM_MOUSEACTIVATE = 0x0021
	WM_NCHITTEST     = 0x0084
	WM_KEYDOWN       = 0x0100
	WM_HOTKEY        = 0x0312

	MA_NOACTIVATE = 3

	MOD_ALT      = 0x1
	MOD_CONTROL  = 0x2
	MOD_SHIFT    = 0x4
	MOD_NOREPEAT = 0x4000

	VK_ESCAPE = 0x1B

	WDA_NONE               = 0x0
	WDA_EXCLUDEFROMCAPTURE = 0x11

	THREAD_PRIORITY_HIGHEST = 2

	WAIT_OBJECT_0 = 0
	WAIT_TIMEOUT  = 0x102
	INFINITE      = 0xFFFFFFFF

	createWaitableTimerHighResolution = 0x2
	timerAllAccess                    = 0x1F0003

	dwmwaCloaked = 14
)

var hwndTopmost = ^uintptr(0) // (HWND)-1

// RECT is a rectangle in pixels (Right/Bottom are exclusive).
type RECT struct{ Left, Top, Right, Bottom int32 }

func (r RECT) W() int      { return int(r.Right - r.Left) }
func (r RECT) H() int      { return int(r.Bottom - r.Top) }
func (r RECT) Empty() bool { return r.Right <= r.Left || r.Bottom <= r.Top }
func (r RECT) String() string {
	return fmt.Sprintf("(%d,%d %dx%d)", r.Left, r.Top, r.W(), r.H())
}

func (r RECT) Contains(x, y int32) bool {
	return x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom
}

// Intersect returns the overlap of two rectangles (may be empty).
func (r RECT) Intersect(o RECT) RECT {
	res := RECT{max(r.Left, o.Left), max(r.Top, o.Top), min(r.Right, o.Right), min(r.Bottom, o.Bottom)}
	if res.Empty() {
		return RECT{}
	}
	return res
}

func (r RECT) Offset(dx, dy int32) RECT {
	return RECT{r.Left + dx, r.Top + dy, r.Right + dx, r.Bottom + dy}
}

type POINT struct{ X, Y int32 }

type MSG struct {
	HWnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       POINT
	LPrivate uint32
}

func lastErr(what string, err error) error {
	var en syscall.Errno
	if errors.As(err, &en) && en != 0 {
		return fmt.Errorf("%s: %w", what, en)
	}
	return fmt.Errorf("%s failed", what)
}

// SetDPIAware enables Per-Monitor-V2 DPI awareness so every coordinate is in physical pixels.
func SetDPIAware() error {
	if err := procSetProcessDpiAwarenessContext.Find(); err != nil {
		return err
	}
	r, _, err := procSetProcessDpiAwarenessContext.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4
	if r == 0 {
		return lastErr("SetProcessDpiAwarenessContext", err)
	}
	return nil
}

// TimeBeginPeriod / TimeEndPeriod control the system timer's resolution.
func TimeBeginPeriod(ms uint32) { procTimeBeginPeriod.Call(uintptr(ms)) }
func TimeEndPeriod(ms uint32)   { procTimeEndPeriod.Call(uintptr(ms)) }

// SetThreadPriorityHighest raises the current OS thread's priority (the main thread is
// locked via runtime.LockOSThread in main).
func SetThreadPriorityHighest() error {
	h, _, _ := procGetCurrentThread.Call()
	r, _, err := procSetThreadPriority.Call(h, THREAD_PRIORITY_HIGHEST)
	if r == 0 {
		return lastErr("SetThreadPriority", err)
	}
	return nil
}

// RegisterHotKey registers a global hotkey for the current thread (hwnd = 0):
// WM_HOTKEY lands in the thread's queue and is handled in PumpMessages.
func RegisterHotKey(id int, mods, vk uint32) error {
	r, _, err := procRegisterHotKey.Call(0, uintptr(id), uintptr(mods|MOD_NOREPEAT), uintptr(vk))
	if r == 0 {
		return lastErr(fmt.Sprintf("RegisterHotKey(id=%d)", id), err)
	}
	return nil
}

func UnregisterHotKey(id int) { procUnregisterHotKey.Call(0, uintptr(id)) }

func CloseHandle(h uintptr) {
	if h != 0 {
		procCloseHandle.Call(h)
	}
}

func WaitForSingleObject(h uintptr, ms uint32) uint32 {
	r, _, _ := procWaitForSingleObject.Call(h, uintptr(ms))
	return uint32(r)
}

func IsWindow(hwnd uintptr) bool {
	r, _, _ := procIsWindow.Call(hwnd)
	return r != 0
}

func IsIconic(hwnd uintptr) bool {
	r, _, _ := procIsIconic.Call(hwnd)
	return r != 0
}

// ClientRectOnScreen returns the window's client area in screen (virtual desktop) coordinates.
func ClientRectOnScreen(hwnd uintptr) (RECT, error) {
	var rc RECT
	r, _, err := procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	if r == 0 {
		return RECT{}, lastErr("GetClientRect", err)
	}
	var pt POINT
	r, _, err = procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
	if r == 0 {
		return RECT{}, lastErr("ClientToScreen", err)
	}
	return rc.Offset(pt.X, pt.Y), nil
}

// AdjustWindowRect returns the outer window rectangle for style style such that the
// client area equals client.
func AdjustWindowRect(client RECT, style, exStyle uint32) RECT {
	r := client
	procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&r)), uintptr(style), 0, uintptr(exStyle))
	return r
}

func moduleHandle() uintptr {
	h, _, _ := procGetModuleHandleW.Call(0)
	return h
}
