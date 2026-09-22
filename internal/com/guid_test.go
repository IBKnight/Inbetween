package com

import (
	"errors"
	"fmt"
	"testing"
)

func TestParseGUID(t *testing.T) {
	g := MustGUID("{6f15aaf2-d208-4e89-9ab4-489535d34f9c}") // ID3D11Texture2D
	if g.Data1 != 0x6f15aaf2 || g.Data2 != 0xd208 || g.Data3 != 0x4e89 {
		t.Fatalf("bad head: %+v", g)
	}
	want := [8]byte{0x9a, 0xb4, 0x48, 0x95, 0x35, 0xd3, 0x4f, 0x9c}
	if g.Data4 != want {
		t.Fatalf("bad tail: %x", g.Data4)
	}
	if _, err := ParseGUID("nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestHRESULT(t *testing.T) {
	if !DXGI_ERROR_WAIT_TIMEOUT.Failed() || S_FALSE.Failed() {
		t.Fatal("Failed() is wrong")
	}
	err := Check(0x887A0027, "AcquireNextFrame")
	if !errors.Is(err, DXGI_ERROR_WAIT_TIMEOUT) {
		t.Fatalf("errors.Is failed: %v", err)
	}
	if got := fmt.Sprint(HR(0x887A0026)); got != "DXGI_ERROR_ACCESS_LOST (0x887A0026)" {
		t.Fatalf("bad string %q", got)
	}
}
