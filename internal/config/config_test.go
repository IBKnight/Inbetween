package config

import (
	"io"
	"testing"
)

func TestDefaults(t *testing.T) {
	c, err := Parse(nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != "live" || c.W != 1280 || c.H != 720 || c.Mult != 2 {
		t.Fatalf("%+v", c)
	}
	if o := c.FGOptions(); o.Radius != c.Radius || o.FlowScale != c.FlowScale {
		t.Fatal("FGOptions mismatch")
	}
}

func TestParseErrors(t *testing.T) {
	bad := [][]string{
		{"-mode", "nope"},
		{"-algo", "dlss"},
		{"-size", "10x"},
		{"-rect", "1,2,3"},
		{"-mode", "offline"},
		{"-mult", "0"},
	}
	for _, a := range bad {
		if _, err := Parse(a, io.Discard); err == nil {
			t.Errorf("expected error for %v", a)
		}
	}
}

func TestParseRect(t *testing.T) {
	x, y, w, h, err := ParseRect("-1920, 0, 1920,1080")
	if err != nil || x != -1920 || y != 0 || w != 1920 || h != 1080 {
		t.Fatal(x, y, w, h, err)
	}
}
