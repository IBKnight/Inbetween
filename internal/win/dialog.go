//go:build windows

package win

import (
	"syscall"
	"unsafe"
)

// A minimal set of native Win32 controls for a single-purpose launcher dialog (see
// internal/app/gui.go). Deliberately narrow: static labels, buttons, radio groups, and a
// drop-down list — enough for one simple screen, nothing more.

var (
	gdi32 = syscall.NewLazyDLL("gdi32.dll")

	procSendMessageW    = user32.NewProc("SendMessageW")
	procSetWindowTextW  = user32.NewProc("SetWindowTextW")
	procGetMessageW     = user32.NewProc("GetMessageW")
	procPostQuitMessage = user32.NewProc("PostQuitMessage")
	procGetStockObject  = gdi32.NewProc("GetStockObject")
)

const (
	wmCommand = 0x0111
	wmSetFont = 0x0030

	wsChild   = 0x40000000
	wsVisible = 0x10000000
	wsGroup   = 0x00020000
	// WS_CAPTION | WS_SYSMENU: a title bar with a close button, not resizable, no
	// minimize/maximize boxes — a fixed-size utility window.
	dialogStyle = 0x00C00000 | 0x00080000

	bsAutoRadioButton = 0x00000009
	bsAutoCheckbox    = 0x00000003
	bsDefPushButton   = 0x00000001
	bsMultiline       = 0x00002000

	cbsDropDownList = 0x0003
	cbAddString     = 0x0143
	cbResetContent  = 0x014B
	cbGetCurSel     = 0x0147
	cbSetCurSel     = 0x014E

	bmGetCheck = 0x00F0
	bmSetCheck = 0x00F1
	bstChecked = 1

	defaultGUIFont = 17 // DEFAULT_GUI_FONT, for GetStockObject
)

const dialogClassName = "InbetweenDialog"

var (
	dialogClassRegistered bool
	dialogWndProcCB       = syscall.NewCallback(dialogWndProc)
	dialogCommand         func(id int, notify uint16)
	dialogFont            uintptr
)

func dialogWndProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch uint32(msg) {
	case wmCommand:
		if dialogCommand != nil {
			dialogCommand(int(wp&0xFFFF), uint16(wp>>16))
		}
		return 0
	case WM_CLOSE:
		procDestroyWindow.Call(hwnd)
		return 0
	case WM_DESTROY:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

func registerDialogClass() error {
	if dialogClassRegistered {
		return nil
	}
	name, _ := syscall.UTF16PtrFromString(dialogClassName)
	cursor, _, _ := procLoadCursorW.Call(0, 32512) // IDC_ARROW
	wc := wndClassEx{
		LpfnWndProc:   dialogWndProcCB,
		HInstance:     moduleHandle(),
		HCursor:       cursor,
		HbrBackground: 16, // COLOR_BTNFACE + 1: standard dialog-gray background
		LpszClassName: name,
	}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if r == 0 {
		return lastErr("RegisterClassExW(dialog)", err)
	}
	dialogClassRegistered = true
	return nil
}

// Dialog is a small fixed-size top-level window hosting plain Win32 controls.
type Dialog struct {
	HWND uintptr
}

// NewDialog creates and shows a w×h (client area) top-level window titled title.
func NewDialog(title string, w, h int) (*Dialog, error) {
	if err := registerDialogClass(); err != nil {
		return nil, err
	}
	if dialogFont == 0 {
		dialogFont, _, _ = procGetStockObject.Call(defaultGUIFont)
	}
	outer := AdjustWindowRect(RECT{0, 0, int32(w), int32(h)}, dialogStyle, 0)
	cls, _ := syscall.UTF16PtrFromString(dialogClassName)
	t, _ := syscall.UTF16PtrFromString(title)
	hwnd, _, err := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(t)), dialogStyle,
		uintptr(0x80000000), uintptr(0x80000000), uintptr(outer.W()), uintptr(outer.H()), // CW_USEDEFAULT position
		0, 0, moduleHandle(), 0)
	if hwnd == 0 {
		return nil, lastErr("CreateWindowExW(dialog)", err)
	}
	procShowWindow.Call(hwnd, SW_SHOW)
	return &Dialog{HWND: hwnd}, nil
}

// OnCommand sets the callback invoked for every WM_COMMAND (button clicks etc.) the
// dialog or any of its child controls receives. Only one Dialog is ever live at a time
// in this app, so a single package-level callback is enough.
func (d *Dialog) OnCommand(fn func(id int, notify uint16)) { dialogCommand = fn }

func (d *Dialog) control(class, text string, style uint32, id, x, y, w, h int) uintptr {
	cls, _ := syscall.UTF16PtrFromString(class)
	t, _ := syscall.UTF16PtrFromString(text)
	hwnd, _, _ := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(t)), uintptr(style|wsChild|wsVisible),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h), d.HWND, uintptr(id), moduleHandle(), 0)
	if hwnd != 0 && dialogFont != 0 {
		procSendMessageW.Call(hwnd, wmSetFont, dialogFont, 1)
	}
	return hwnd
}

// AddStatic adds a plain (word-wrapping) text label.
func (d *Dialog) AddStatic(text string, x, y, w, h int) uintptr {
	return d.control("STATIC", text, 0, 0, x, y, w, h)
}

// AddButton adds a push button with control id id.
func (d *Dialog) AddButton(text string, id, x, y, w, h int, isDefault bool) uintptr {
	var style uint32
	if isDefault {
		style = bsDefPushButton
	}
	return d.control("BUTTON", text, style, id, x, y, w, h)
}

// AddRadio adds a radio button with control id id. group starts a new mutually-exclusive
// group — set it on the first radio button of each group only.
func (d *Dialog) AddRadio(text string, id, x, y, w, h int, group bool) uintptr {
	style := uint32(bsAutoRadioButton)
	if group {
		style |= wsGroup
	}
	return d.control("BUTTON", text, style, id, x, y, w, h)
}

// AddCheckbox adds an independent (non-exclusive) toggle with control id id. The label
// word-wraps within w, so h should fit as many lines as the text needs.
func (d *Dialog) AddCheckbox(text string, id, x, y, w, h int) uintptr {
	return d.control("BUTTON", text, bsAutoCheckbox|bsMultiline, id, x, y, w, h)
}

// AddCombo adds a drop-down list (the text can't be typed in, only picked). h is the
// height available for the dropped-down list, not the closed box — Win32 sizes the
// closed box itself from the font.
func (d *Dialog) AddCombo(id, x, y, w, h int) uintptr {
	return d.control("COMBOBOX", "", cbsDropDownList, id, x, y, w, h)
}

func (d *Dialog) ComboReset(hwnd uintptr) { procSendMessageW.Call(hwnd, cbResetContent, 0, 0) }

func (d *Dialog) ComboAdd(hwnd uintptr, text string) {
	t, _ := syscall.UTF16PtrFromString(text)
	procSendMessageW.Call(hwnd, cbAddString, 0, uintptr(unsafe.Pointer(t)))
}

func (d *Dialog) ComboSelect(hwnd uintptr, idx int) {
	procSendMessageW.Call(hwnd, cbSetCurSel, uintptr(idx), 0)
}

// ComboSelected returns the selected item's index, or -1 if none is selected.
func (d *Dialog) ComboSelected(hwnd uintptr) int {
	r, _, _ := procSendMessageW.Call(hwnd, cbGetCurSel, 0, 0)
	return int(int32(r))
}

func (d *Dialog) SetChecked(hwnd uintptr, checked bool) {
	var v uintptr
	if checked {
		v = bstChecked
	}
	procSendMessageW.Call(hwnd, bmSetCheck, v, 0)
}

func (d *Dialog) IsChecked(hwnd uintptr) bool {
	r, _, _ := procSendMessageW.Call(hwnd, bmGetCheck, 0, 0)
	return r == bstChecked
}

func (d *Dialog) SetText(hwnd uintptr, text string) {
	t, _ := syscall.UTF16PtrFromString(text)
	procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(t)))
}

// Run pumps messages until the dialog is closed (WM_QUIT). Blocks the calling thread.
func (d *Dialog) Run() {
	var m MSG
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (d *Dialog) Destroy() {
	if d.HWND != 0 {
		procDestroyWindow.Call(d.HWND)
		d.HWND = 0
	}
}
