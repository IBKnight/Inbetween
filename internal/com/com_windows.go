//go:build windows

package com

import (
	"syscall"
	"unsafe"
)

// Ptr is a pointer to a COM object (the memory is owned by Windows, not Go).
// The zero value is a nil object.
type Ptr struct{ p unsafe.Pointer }

// FromRaw wraps a raw pointer obtained from an out-parameter.
func FromRaw(p unsafe.Pointer) Ptr { return Ptr{p} }

// Raw returns the raw pointer.
func (o Ptr) Raw() unsafe.Pointer { return o.p }

// U returns the pointer as a uintptr for passing as a Call/syscall argument.
// Safe: a COM object's memory isn't managed by the Go garbage collector.
func (o Ptr) U() uintptr { return uintptr(o.p) }

// Nil reports whether the object was never created or has already been released.
func (o Ptr) Nil() bool { return o.p == nil }

// Call invokes the vtable method at index idx; this is passed automatically.
//
// IMPORTANT (safety rule): pass pointers to Go memory ONLY like this:
//
//	obj.Call(idx, uintptr(unsafe.Pointer(&x)))
//
// — convert it directly inline in the call expression. The uintptrescapes directive
// guarantees x is moved to the heap and stays alive until the call returns. Never
// store uintptr(unsafe.Pointer(&x)) in a variable ahead of the call.
//
//go:uintptrescapes
func (o Ptr) Call(idx int, args ...uintptr) uintptr {
	if o.p == nil {
		panic("com: method call on nil object")
	}
	vtbl := *(*unsafe.Pointer)(o.p)
	fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	var buf [20]uintptr
	buf[0] = uintptr(o.p)
	n := copy(buf[1:], args)
	r, _, _ := syscall.SyscallN(fn, buf[:n+1]...)
	return r
}

// AddRef increments the reference count.
func (o Ptr) AddRef() {
	if o.p != nil {
		o.Call(1)
	}
}

// Release releases the reference and clears Ptr. Safe to call more than once.
func (o *Ptr) Release() {
	if o.p != nil {
		o.Call(2)
		o.p = nil
	}
}

// QueryInterface requests a different interface on the same object (a new reference).
func (o Ptr) QueryInterface(iid *GUID) (Ptr, error) {
	var out unsafe.Pointer
	r := o.Call(0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	if err := Check(r, "QueryInterface "+iid.String()); err != nil {
		return Ptr{}, err
	}
	return Ptr{out}, nil
}
