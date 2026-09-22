package gfx

import (
	"testing"
	"unsafe"
)

// If this test fails, Params changed without syncing shaders/common.hlsli.
func TestParamsLayout(t *testing.T) {
	var p Params
	if s := unsafe.Sizeof(p); s != 112 {
		t.Fatalf("sizeof(Params) = %d, want 112", s)
	}
	checks := map[string][2]uintptr{
		"DstSize": {unsafe.Offsetof(p.DstSize), 16},
		"T":       {unsafe.Offsetof(p.T), 32},
		"Flags":   {unsafe.Offsetof(p.Flags), 48},
		"User":    {unsafe.Offsetof(p.User), 64},
		"User2":   {unsafe.Offsetof(p.User2), 80},
		"User3":   {unsafe.Offsetof(p.User3), 96},
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("offset(%s) = %d, want %d", name, c[0], c[1])
		}
	}
}
