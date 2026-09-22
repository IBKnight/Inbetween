//go:build windows

package gfx

import (
	"unsafe"

	"inbetween/internal/com"
	"inbetween/internal/win"
)

// DXGI_FORMAT
const (
	FormatUnknown uint32 = 0
	FormatRGBA32F uint32 = 2
	FormatRGBA16F uint32 = 10
	FormatRG32F   uint32 = 16
	FormatRGBA8   uint32 = 28 // R8G8B8A8_UNORM
	FormatRG16F   uint32 = 34
	FormatR32F    uint32 = 41
	FormatR16F    uint32 = 54
	FormatR8      uint32 = 61
	FormatBGRA8   uint32 = 87 // B8G8R8A8_UNORM (the desktop's format)
)

// BytesPerPixel for the supported formats (0 = unknown).
func BytesPerPixel(f uint32) int {
	switch f {
	case FormatRGBA32F:
		return 16
	case FormatRGBA16F, FormatRG32F:
		return 8
	case FormatRGBA8, FormatBGRA8, FormatRG16F, FormatR32F:
		return 4
	case FormatR16F:
		return 2
	case FormatR8:
		return 1
	}
	return 0
}

// D3D11 enums/flags
const (
	BindVertexBuffer    uint32 = 0x1
	BindConstantBuffer  uint32 = 0x4
	BindShaderResource  uint32 = 0x8
	BindRenderTarget    uint32 = 0x20
	BindUnorderedAccess uint32 = 0x80

	UsageDefault   uint32 = 0
	UsageImmutable uint32 = 1
	UsageDynamic   uint32 = 2
	UsageStaging   uint32 = 3

	CPUAccessWrite uint32 = 0x10000
	CPUAccessRead  uint32 = 0x20000

	MapRead         uint32 = 1
	MapWrite        uint32 = 2
	MapReadWrite    uint32 = 3
	MapWriteDiscard uint32 = 4

	FilterPoint  uint32 = 0x00 // D3D11_FILTER_MIN_MAG_MIP_POINT
	FilterLinear uint32 = 0x15 // D3D11_FILTER_MIN_MAG_MIP_LINEAR

	AddressWrap   uint32 = 1
	AddressMirror uint32 = 2
	AddressClamp  uint32 = 3

	ComparisonNever uint32 = 1

	TopologyTriangleList uint32 = 4

	QueryEvent             uint32 = 0
	QueryTimestamp         uint32 = 2
	QueryTimestampDisjoint uint32 = 3

	AsyncGetDataDoNotFlush uint32 = 1

	driverTypeUnknown = 0
	createDeviceDebug = 0x2
	createDeviceBGRA  = 0x20
	featureLevel11_0  = 0xb000
	featureLevel11_1  = 0xb100
	d3d11SDKVersion   = 7
)

// DXGI enums/flags
const (
	usageRenderTargetOutput uint32 = 0x20

	swapEffectFlipSequential uint32 = 3
	swapEffectFlipDiscard    uint32 = 4

	scalingStretch uint32 = 0

	alphaModeUnspecified uint32 = 0

	swapChainFlagFrameLatencyWaitable uint32 = 64
	swapChainFlagAllowTearing         uint32 = 2048

	presentAllowTearing uint32 = 0x200

	mwaNoAltEnter uint32 = 0x2

	featurePresentAllowTearing = 0

	ModeRotationIdentity uint32 = 1
)

// Structs (layout matches C on amd64/arm64)

type SampleDesc struct{ Count, Quality uint32 }

type Texture2DDesc struct {
	Width, Height, MipLevels, ArraySize, Format uint32
	SampleDesc                                  SampleDesc
	Usage, BindFlags, CPUAccessFlags, MiscFlags uint32
}

type BufferDesc struct {
	ByteWidth, Usage, BindFlags, CPUAccessFlags, MiscFlags, StructureByteStride uint32
}

type MappedSubresource struct {
	PData                unsafe.Pointer
	RowPitch, DepthPitch uint32
}

type Box struct{ Left, Top, Front, Right, Bottom, Back uint32 }

type Viewport struct{ TopLeftX, TopLeftY, Width, Height, MinDepth, MaxDepth float32 }

type SamplerDesc struct {
	Filter, AddressU, AddressV, AddressW uint32
	MipLODBias                           float32
	MaxAnisotropy, ComparisonFunc        uint32
	BorderColor                          [4]float32
	MinLOD, MaxLOD                       float32
}

type QueryDesc struct{ Query, MiscFlags uint32 }

type QueryDataTimestampDisjoint struct {
	Frequency uint64
	Disjoint  int32
}

type SwapChainDesc1 struct {
	Width, Height, Format uint32
	Stereo                int32
	SampleDesc            SampleDesc
	BufferUsage           uint32
	BufferCount           uint32
	Scaling               uint32
	SwapEffect            uint32
	AlphaMode             uint32
	Flags                 uint32
}

type ModeDesc struct {
	Width, Height                     uint32
	RefreshNum, RefreshDen            uint32
	Format, ScanlineOrdering, Scaling uint32
}

type OutDuplDesc struct {
	ModeDesc                   ModeDesc
	Rotation                   uint32
	DesktopImageInSystemMemory int32
}

type OutDuplPointerPosition struct {
	Position win.POINT
	Visible  int32
}

// OutDuplFrameInfo = DXGI_OUTDUPL_FRAME_INFO (48 bytes).
type OutDuplFrameInfo struct {
	LastPresentTime           int64  // QPC of the last desktop update; 0 = only the cursor moved
	LastMouseUpdateTime       int64  //
	AccumulatedFrames         uint32 // updates accumulated since the last capture (>1 = drops)
	RectsCoalesced            int32
	ProtectedContentMaskedOut int32
	PointerPosition           OutDuplPointerPosition
	TotalMetadataBufferSize   uint32
	PointerShapeBufferSize    uint32
}

type OutputDesc struct {
	DeviceName         [32]uint16
	DesktopCoordinates win.RECT
	AttachedToDesktop  int32
	Rotation           uint32
	Monitor            uintptr
}

type AdapterDesc1 struct {
	Description                                                     [128]uint16
	VendorID, DeviceID, SubSysID, Revision                          uint32
	DedicatedVideoMemory, DedicatedSystemMemory, SharedSystemMemory uintptr
	LUIDLow                                                         uint32
	LUIDHigh                                                        int32
	Flags                                                           uint32
}

// FrameStatistics is DXGI_FRAME_STATISTICS.
type FrameStatistics struct {
	PresentCount, PresentRefreshCount, SyncRefreshCount uint32
	SyncQPCTime, SyncGPUTime                            int64
}

type d3d11Message struct {
	Category, Severity, ID int32
	PDescription           *byte
	DescriptionByteLength  uintptr
}

type shaderMacro struct{ Name, Definition *byte }

// IID
var (
	IID_IDXGIFactory1   = com.MustGUID("770aae78-f26f-4dba-a829-253c83d1b387")
	IID_IDXGIFactory2   = com.MustGUID("50c83a1c-e072-4c48-87b0-3630fa36a6d0")
	IID_IDXGIFactory5   = com.MustGUID("7632e1f5-ee65-4dca-87fd-84cd75f8838d")
	IID_IDXGIOutput1    = com.MustGUID("00cddea8-939b-4b83-a340-a685226666cc")
	IID_IDXGIDevice1    = com.MustGUID("77db970f-6276-48ba-ba28-070143b4392c")
	IID_IDXGISwapChain2 = com.MustGUID("a8be2ac4-199f-4946-b331-79599fb98de7")
	IID_ID3D11Texture2D = com.MustGUID("6f15aaf2-d208-4e89-9ab4-489535d34f9c")
	IID_ID3D11InfoQueue = com.MustGUID("6543dbb6-1b48-42f5-ab82-e97ec74326f6")
)

// vtable indices
//
// Counted from the start of the vtable: IUnknown = 0..2, then in declaration order from
// the SDK headers, accounting for the inheritance chain. Double-check against
// d3d11.h/dxgi*.h when adding new methods: a wrong index calls the wrong function
// (usually a crash). IDXGIObject adds 3..6 (SetPrivateData, SetPrivateDataInterface,
// GetPrivateData, GetParent); IDXGIDeviceSubObject adds 7 (GetDevice); ID3D11DeviceChild
// occupies 3..6.

// IDXGIFactory -> IDXGIFactory1 -> IDXGIFactory2 -> ... -> IDXGIFactory5
const (
	factoryMakeWindowAssociation   = 8
	factoryEnumAdapters1           = 12
	factory2CreateSwapChainForHwnd = 15
	factory5CheckFeatureSupport    = 28
)

// IDXGIAdapter -> IDXGIAdapter1
const (
	adapterEnumOutputs = 7
	adapterGetDesc1    = 10
)

// IDXGIOutput -> IDXGIOutput1
const (
	outputGetDesc          = 7
	output1DuplicateOutput = 22
)

// IDXGIOutputDuplication
const (
	duplGetDesc            = 7
	duplAcquireNextFrame   = 8
	duplGetFrameDirtyRects = 9
	duplReleaseFrame       = 14
)

// IDXGIDevice -> IDXGIDevice1
const (
	dxgiDeviceSetGPUThreadPriority    = 10
	dxgiDevice1SetMaximumFrameLatency = 12
)

// IDXGISwapChain -> IDXGISwapChain1 -> IDXGISwapChain2
const (
	scPresent                        = 8
	scGetBuffer                      = 9
	scResizeBuffers                  = 13
	scGetFrameStatistics             = 16
	sc2SetMaximumFrameLatency        = 31
	sc2GetFrameLatencyWaitableObject = 33
)

// ID3D11Device
const (
	devCreateBuffer              = 3
	devCreateTexture2D           = 5
	devCreateShaderResourceView  = 7
	devCreateUnorderedAccessView = 8
	devCreateRenderTargetView    = 9
	devCreateVertexShader        = 12
	devCreatePixelShader         = 15
	devCreateComputeShader       = 18
	devCreateSamplerState        = 23
	devCreateQuery               = 24
	devGetFeatureLevel           = 37
	devGetDeviceRemovedReason    = 39
)

// ID3D11DeviceContext (ID3D11DeviceChild occupies 3..6)
const (
	ctxVSSetConstantBuffers     = 7
	ctxPSSetShaderResources     = 8
	ctxPSSetShader              = 9
	ctxPSSetSamplers            = 10
	ctxVSSetShader              = 11
	ctxDraw                     = 13
	ctxMap                      = 14
	ctxUnmap                    = 15
	ctxPSSetConstantBuffers     = 16
	ctxIASetInputLayout         = 17
	ctxIASetPrimitiveTopology   = 24
	ctxBegin                    = 27
	ctxEnd                      = 28
	ctxGetData                  = 29
	ctxOMSetRenderTargets       = 33
	ctxDispatch                 = 41
	ctxRSSetViewports           = 44
	ctxCopySubresourceRegion    = 46
	ctxCopyResource             = 47
	ctxUpdateSubresource        = 48
	ctxClearRenderTargetView    = 50
	ctxCSSetShaderResources     = 67
	ctxCSSetUnorderedAccessView = 68
	ctxCSSetShader              = 69
	ctxCSSetSamplers            = 70
	ctxCSSetConstantBuffers     = 71
	ctxClearState               = 110
	ctxFlush                    = 111
)

// ID3D11Resource -> ID3D11Texture2D
const tex2DGetDesc = 10

// ID3D10Blob
const (
	blobGetBufferPointer = 3
	blobGetBufferSize    = 4
)

// ID3D11InfoQueue
const (
	infoSetMessageCountLimit = 3
	infoClearStoredMessages  = 4
	infoGetMessage           = 5
	infoGetNumStoredMessages = 8
)
