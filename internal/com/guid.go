// Package com is a minimal binding for calling COM methods by vtable index, without cgo.
//
// How it works: a COM object is a pointer to a struct whose first field is a pointer to
// a function table (vtable). The method at index N is invoked as vtable[N](this, args...).
// Indices come from the Windows SDK headers (d3d11.h, dxgi.h, dxgi1_2.h, ...), accounting
// for inheritance: IUnknown occupies 0..2.
package com

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// GUID uses the Windows binary layout.
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// ParseGUID parses a string like "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" (braces allowed).
func ParseGUID(s string) (GUID, error) {
	s = strings.Trim(s, "{}")
	p := strings.Split(s, "-")
	if len(p) != 5 || len(p[0]) != 8 || len(p[1]) != 4 || len(p[2]) != 4 || len(p[3]) != 4 || len(p[4]) != 12 {
		return GUID{}, fmt.Errorf("com: bad GUID %q", s)
	}
	d1, err := strconv.ParseUint(p[0], 16, 32)
	if err != nil {
		return GUID{}, err
	}
	d2, err := strconv.ParseUint(p[1], 16, 16)
	if err != nil {
		return GUID{}, err
	}
	d3, err := strconv.ParseUint(p[2], 16, 16)
	if err != nil {
		return GUID{}, err
	}
	tail, err := hex.DecodeString(p[3] + p[4])
	if err != nil {
		return GUID{}, err
	}
	g := GUID{Data1: uint32(d1), Data2: uint16(d2), Data3: uint16(d3)}
	copy(g.Data4[:], tail)
	return g, nil
}

// MustGUID is ParseGUID that panics on error, for declaring IIDs in var blocks.
func MustGUID(s string) GUID {
	g, err := ParseGUID(s)
	if err != nil {
		panic(err)
	}
	return g
}

func (g GUID) String() string {
	return fmt.Sprintf("%08x-%04x-%04x-%02x%02x-%x", g.Data1, g.Data2, g.Data3, g.Data4[0], g.Data4[1], g.Data4[2:])
}

// HRESULT is a COM result code. Implements error.
type HRESULT int32

// HR converts a call's raw return value into an HRESULT.
func HR(r uintptr) HRESULT { return HRESULT(int32(uint32(r))) }

func hr(u uint32) HRESULT { return HRESULT(int32(u)) }

// Failed reports whether the sign bit is set.
func (h HRESULT) Failed() bool { return h < 0 }

func (h HRESULT) Error() string {
	if n, ok := hrNames[h]; ok {
		return fmt.Sprintf("%s (0x%08X)", n, uint32(h))
	}
	return fmt.Sprintf("HRESULT 0x%08X", uint32(h))
}

// Check returns a contextual error if r is a failed HRESULT.
func Check(r uintptr, what string) error {
	if h := HR(r); h.Failed() {
		return fmt.Errorf("%s: %w", what, h)
	}
	return nil
}

var (
	S_OK    = hr(0)
	S_FALSE = hr(1)

	E_NOTIMPL      = hr(0x80004001)
	E_NOINTERFACE  = hr(0x80004002)
	E_FAIL         = hr(0x80004005)
	E_ACCESSDENIED = hr(0x80070005)
	E_INVALIDARG   = hr(0x80070057)
	E_OUTOFMEMORY  = hr(0x8007000E)

	DXGI_ERROR_INVALID_CALL            = hr(0x887A0001)
	DXGI_ERROR_NOT_FOUND               = hr(0x887A0002)
	DXGI_ERROR_UNSUPPORTED             = hr(0x887A0004)
	DXGI_ERROR_DEVICE_REMOVED          = hr(0x887A0005)
	DXGI_ERROR_DEVICE_HUNG             = hr(0x887A0006)
	DXGI_ERROR_DEVICE_RESET            = hr(0x887A0007)
	DXGI_ERROR_DRIVER_INTERNAL_ERROR   = hr(0x887A0020)
	DXGI_ERROR_NOT_CURRENTLY_AVAILABLE = hr(0x887A0022)
	DXGI_ERROR_ACCESS_LOST             = hr(0x887A0026)
	DXGI_ERROR_WAIT_TIMEOUT            = hr(0x887A0027)
	DXGI_ERROR_SESSION_DISCONNECTED    = hr(0x887A0028)

	DXGI_STATUS_OCCLUDED = hr(0x087A0001)
)

var hrNames = map[HRESULT]string{
	S_FALSE:                            "S_FALSE",
	E_NOTIMPL:                          "E_NOTIMPL",
	E_NOINTERFACE:                      "E_NOINTERFACE",
	E_FAIL:                             "E_FAIL",
	E_ACCESSDENIED:                     "E_ACCESSDENIED",
	E_INVALIDARG:                       "E_INVALIDARG",
	E_OUTOFMEMORY:                      "E_OUTOFMEMORY",
	DXGI_ERROR_INVALID_CALL:            "DXGI_ERROR_INVALID_CALL",
	DXGI_ERROR_NOT_FOUND:               "DXGI_ERROR_NOT_FOUND",
	DXGI_ERROR_UNSUPPORTED:             "DXGI_ERROR_UNSUPPORTED",
	DXGI_ERROR_DEVICE_REMOVED:          "DXGI_ERROR_DEVICE_REMOVED",
	DXGI_ERROR_DEVICE_HUNG:             "DXGI_ERROR_DEVICE_HUNG",
	DXGI_ERROR_DEVICE_RESET:            "DXGI_ERROR_DEVICE_RESET",
	DXGI_ERROR_DRIVER_INTERNAL_ERROR:   "DXGI_ERROR_DRIVER_INTERNAL_ERROR",
	DXGI_ERROR_NOT_CURRENTLY_AVAILABLE: "DXGI_ERROR_NOT_CURRENTLY_AVAILABLE",
	DXGI_ERROR_ACCESS_LOST:             "DXGI_ERROR_ACCESS_LOST",
	DXGI_ERROR_WAIT_TIMEOUT:            "DXGI_ERROR_WAIT_TIMEOUT",
	DXGI_ERROR_SESSION_DISCONNECTED:    "DXGI_ERROR_SESSION_DISCONNECTED",
	DXGI_STATUS_OCCLUDED:               "DXGI_STATUS_OCCLUDED",
}
