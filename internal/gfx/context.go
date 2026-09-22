//go:build windows

package gfx

import (
	"unsafe"

	"inbetween/internal/com"
)

const maxSlots = 8

// Bindings are a pass's resources. A slice index is the register number (t0.., u0.., b0..,
// s0..). nil elements bind as null.
type Bindings struct {
	CB       []*ConstantBuffer
	SRV      []*Texture
	UAV      []*Texture
	Samplers []com.Ptr
}

func srvArray(ts []*Texture) (a [maxSlots]uintptr, n int) {
	for i, t := range ts {
		if i >= maxSlots {
			break
		}
		if t != nil {
			a[i] = t.SRV.U()
		}
		n = i + 1
	}
	return
}

func uavArray(ts []*Texture) (a [maxSlots]uintptr, n int) {
	for i, t := range ts {
		if i >= maxSlots {
			break
		}
		if t != nil {
			a[i] = t.UAV.U()
		}
		n = i + 1
	}
	return
}

func cbArray(cs []*ConstantBuffer) (a [maxSlots]uintptr, n int) {
	for i, c := range cs {
		if i >= maxSlots {
			break
		}
		if c != nil {
			a[i] = c.Buf.U()
		}
		n = i + 1
	}
	return
}

func ptrArray(ps []com.Ptr) (a [maxSlots]uintptr, n int) {
	for i, p := range ps {
		if i >= maxSlots {
			break
		}
		a[i] = p.U()
		n = i + 1
	}
	return
}

// RunCompute dispatches a compute shader over a width×height grid of threads (group
// counts come from s.GroupX/GroupY), then unbinds SRV/UAV so the next pass can read
// what this one just wrote.
func (d *Device) RunCompute(s *Shader, width, height int, b Bindings) {
	if s == nil || s.Obj.Nil() || width <= 0 || height <= 0 {
		return
	}
	if d.Prof != nil {
		defer d.Prof.Scope(s.File)()
	}
	ctx := d.Ctx
	ctx.Call(ctxCSSetShader, s.Obj.U(), 0, 0)
	if a, n := cbArray(b.CB); n > 0 {
		ctx.Call(ctxCSSetConstantBuffers, 0, uintptr(n), uintptr(unsafe.Pointer(&a[0])))
	}
	if a, n := ptrArray(b.Samplers); n > 0 {
		ctx.Call(ctxCSSetSamplers, 0, uintptr(n), uintptr(unsafe.Pointer(&a[0])))
	}
	srv, nsrv := srvArray(b.SRV)
	if nsrv > 0 {
		ctx.Call(ctxCSSetShaderResources, 0, uintptr(nsrv), uintptr(unsafe.Pointer(&srv[0])))
	}
	uav, nuav := uavArray(b.UAV)
	if nuav > 0 {
		ctx.Call(ctxCSSetUnorderedAccessView, 0, uintptr(nuav), uintptr(unsafe.Pointer(&uav[0])), 0)
	}
	gx := (width + s.GroupX - 1) / s.GroupX
	gy := (height + s.GroupY - 1) / s.GroupY
	ctx.Call(ctxDispatch, uintptr(gx), uintptr(gy), 1)

	var null [maxSlots]uintptr
	if nsrv > 0 {
		ctx.Call(ctxCSSetShaderResources, 0, uintptr(nsrv), uintptr(unsafe.Pointer(&null[0])))
	}
	if nuav > 0 {
		ctx.Call(ctxCSSetUnorderedAccessView, 0, uintptr(nuav), uintptr(unsafe.Pointer(&null[0])), 0)
	}
}

// DrawFullscreen draws a fullscreen triangle (vs+ps) into rtv, sized w×h. The pixel
// shader receives b.CB/b.SRV/b.Samplers. Used for presenting to the swapchain.
func (d *Device) DrawFullscreen(vs, ps *Shader, rtv com.Ptr, w, h int, b Bindings) {
	if vs == nil || ps == nil || vs.Obj.Nil() || ps.Obj.Nil() {
		return
	}
	if d.Prof != nil {
		defer d.Prof.Scope("present")()
	}
	ctx := d.Ctx
	rt := [1]uintptr{rtv.U()}
	ctx.Call(ctxOMSetRenderTargets, 1, uintptr(unsafe.Pointer(&rt[0])), 0)
	vp := Viewport{Width: float32(w), Height: float32(h), MaxDepth: 1}
	ctx.Call(ctxRSSetViewports, 1, uintptr(unsafe.Pointer(&vp)))
	ctx.Call(ctxIASetInputLayout, 0)
	ctx.Call(ctxIASetPrimitiveTopology, uintptr(TopologyTriangleList))
	ctx.Call(ctxVSSetShader, vs.Obj.U(), 0, 0)
	ctx.Call(ctxPSSetShader, ps.Obj.U(), 0, 0)
	if a, n := cbArray(b.CB); n > 0 {
		ctx.Call(ctxPSSetConstantBuffers, 0, uintptr(n), uintptr(unsafe.Pointer(&a[0])))
		ctx.Call(ctxVSSetConstantBuffers, 0, uintptr(n), uintptr(unsafe.Pointer(&a[0])))
	}
	if a, n := ptrArray(b.Samplers); n > 0 {
		ctx.Call(ctxPSSetSamplers, 0, uintptr(n), uintptr(unsafe.Pointer(&a[0])))
	}
	srv, nsrv := srvArray(b.SRV)
	if nsrv > 0 {
		ctx.Call(ctxPSSetShaderResources, 0, uintptr(nsrv), uintptr(unsafe.Pointer(&srv[0])))
	}
	ctx.Call(ctxDraw, 3, 0)

	var null [maxSlots]uintptr
	if nsrv > 0 {
		ctx.Call(ctxPSSetShaderResources, 0, uintptr(nsrv), uintptr(unsafe.Pointer(&null[0])))
	}
	ctx.Call(ctxOMSetRenderTargets, 0, 0, 0)
}

func (d *Device) ClearRTV(rtv com.Ptr, r, g, b, a float32) {
	c := [4]float32{r, g, b, a}
	d.Ctx.Call(ctxClearRenderTargetView, rtv.U(), uintptr(unsafe.Pointer(&c[0])))
}
