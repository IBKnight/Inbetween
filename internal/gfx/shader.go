//go:build windows

package gfx

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"inbetween/internal/com"
)

var (
	modCompiler    = syscall.NewLazyDLL("d3dcompiler_47.dll") // ships with Windows 10+ out of the box
	procD3DCompile = modCompiler.NewProc("D3DCompile")
)

const (
	compileDebug            = 1 << 0
	compileSkipOptimization = 1 << 2
	compileEnableStrictness = 1 << 11
	compileOptimization3    = 1 << 15
)

type ShaderStage int

const (
	StageCompute ShaderStage = iota
	StageVertex
	StagePixel
)

func (s ShaderStage) target() string {
	switch s {
	case StageVertex:
		return "vs_5_0"
	case StagePixel:
		return "ps_5_0"
	}
	return "cs_5_0"
}

// Shader is a compiled shader. On hot reload, Obj is swapped in place, so holding onto a
// *Shader for as long as you like is safe.
type Shader struct {
	File    string // file name relative to the shader directory
	Entry   string
	Stage   ShaderStage
	GroupX  int // compute shader group size; passed to HLSL as GROUP_X/GROUP_Y
	GroupY  int
	Defines map[string]string
	Obj     com.Ptr // ID3D11ComputeShader / ID3D11VertexShader / ID3D11PixelShader

	deps  []string // full paths of the file and all its includes
	stamp time.Time
}

// ShaderLib compiles shaders from a directory and recompiles ones that changed (hot reload).
type ShaderLib struct {
	dev     *Device
	Dir     string
	Debug   bool                             // compile with debug info and no optimizations
	Warn    func(format string, args ...any) // where compiler warnings go
	shaders []*Shader
}

func NewShaderLib(d *Device, dir string, debug bool) (*ShaderLib, error) {
	if err := procD3DCompile.Find(); err != nil {
		return nil, fmt.Errorf("d3dcompiler_47.dll не найден: %w", err)
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("каталог шейдеров %q не найден (запускайте из корня проекта или задайте -shaders)", dir)
	}
	return &ShaderLib{dev: d, Dir: dir, Debug: debug, Warn: func(string, ...any) {}}, nil
}

// Compute compiles a compute shader (entry point main, 8×8 group).
func (l *ShaderLib) Compute(file string, defines map[string]string) (*Shader, error) {
	s := &Shader{File: file, Entry: "main", Stage: StageCompute, GroupX: 8, GroupY: 8, Defines: defines}
	return s, l.add(s)
}

// Graphics compiles a vertex/pixel shader with entry point entry.
func (l *ShaderLib) Graphics(file, entry string, stage ShaderStage) (*Shader, error) {
	s := &Shader{File: file, Entry: entry, Stage: stage, GroupX: 1, GroupY: 1}
	return s, l.add(s)
}

func (l *ShaderLib) add(s *Shader) error {
	if err := l.build(s); err != nil {
		return err
	}
	l.shaders = append(l.shaders, s)
	return nil
}

// ReloadChanged recompiles shaders whose file or an include changed.
// On a compile error, the old version keeps running.
func (l *ShaderLib) ReloadChanged() (reloaded int, errs []error) {
	for _, s := range l.shaders {
		if !s.changed() {
			continue
		}
		if err := l.build(s); err != nil {
			errs = append(errs, err)
			s.stamp = time.Now() // don't hammer the compiler until the file changes again
			continue
		}
		reloaded++
	}
	return
}

func (s *Shader) changed() bool {
	for _, p := range s.deps {
		if st, err := os.Stat(p); err == nil && st.ModTime().After(s.stamp) {
			return true
		}
	}
	return false
}

func (l *ShaderLib) Release() {
	for _, s := range l.shaders {
		s.Obj.Release()
	}
	l.shaders = nil
}

func (l *ShaderLib) build(s *Shader) error {
	var deps []string
	src, err := l.preprocess(s.File, map[string]bool{}, &deps)
	if err != nil {
		return err
	}
	stamp := time.Time{}
	for _, p := range deps {
		if st, err := os.Stat(p); err == nil && st.ModTime().After(stamp) {
			stamp = st.ModTime()
		}
	}
	defs := map[string]string{}
	for k, v := range s.Defines {
		defs[k] = v
	}
	if s.Stage == StageCompute {
		defs["GROUP_X"] = fmt.Sprint(s.GroupX)
		defs["GROUP_Y"] = fmt.Sprint(s.GroupY)
	}
	code, err := l.compile(src, s.File, s.Entry, s.Stage.target(), defs)
	if err != nil {
		return err
	}
	obj, err := l.create(s.Stage, code)
	if err != nil {
		return fmt.Errorf("%s: %w", s.File, err)
	}
	s.Obj.Release()
	s.Obj, s.deps, s.stamp = obj, deps, stamp
	return nil
}

// preprocess expands #include "file" (each file at most once) and inserts #line directives
// so compiler errors point at the right file and line.
func (l *ShaderLib) preprocess(rel string, seen map[string]bool, deps *[]string) (string, error) {
	full := filepath.Join(l.Dir, rel)
	if seen[full] {
		return "", nil
	}
	seen[full] = true
	*deps = append(*deps, full)
	f, err := os.Open(full)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var sb strings.Builder
	fmt.Fprintf(&sb, "#line 1 \"%s\"\n", rel)
	sc := bufio.NewScanner(f)
	line := 0
	for sc.Scan() {
		line++
		t := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(t, "#include") {
			q := strings.Trim(strings.TrimSpace(strings.TrimPrefix(t, "#include")), "\"<>")
			inc := filepath.ToSlash(filepath.Join(filepath.Dir(rel), q))
			body, err := l.preprocess(inc, seen, deps)
			if err != nil {
				return "", fmt.Errorf("%s:%d: %w", rel, line, err)
			}
			sb.WriteString(body)
			fmt.Fprintf(&sb, "\n#line %d \"%s\"\n", line+1, rel)
			continue
		}
		sb.WriteString(sc.Text())
		sb.WriteByte('\n')
	}
	return sb.String(), sc.Err()
}

func blobBytes(b com.Ptr) []byte {
	p := b.Call(blobGetBufferPointer)
	n := b.Call(blobGetBufferSize)
	if p == 0 || n == 0 {
		return nil
	}
	// p points into d3dcompiler's memory (not Go's), so copy it out right away.
	// The double cast through &p is the idiom for an address that isn't a Go heap pointer
	// (keeps go vet quiet).
	src := unsafe.Slice(*(**byte)(unsafe.Pointer(&p)), int(n))
	return append([]byte(nil), src...)
}

func (l *ShaderLib) compile(src, name, entry, target string, defines map[string]string) ([]byte, error) {
	keys := make([]string, 0, len(defines))
	for k := range defines {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	macros := make([]shaderMacro, 0, len(keys)+1)
	for _, k := range keys {
		n, _ := syscall.BytePtrFromString(k)
		v, _ := syscall.BytePtrFromString(defines[k])
		macros = append(macros, shaderMacro{n, v})
	}
	macros = append(macros, shaderMacro{}) // {NULL, NULL} terminator

	flags := uint32(compileEnableStrictness)
	if l.Debug {
		flags |= compileDebug | compileSkipOptimization
	} else {
		flags |= compileOptimization3
	}
	srcB := []byte(src)
	nameC, _ := syscall.BytePtrFromString(name)
	entryC, _ := syscall.BytePtrFromString(entry)
	targetC, _ := syscall.BytePtrFromString(target)
	var code, errs unsafe.Pointer
	r, _, _ := procD3DCompile.Call(
		uintptr(unsafe.Pointer(&srcB[0])), uintptr(len(srcB)), uintptr(unsafe.Pointer(nameC)),
		uintptr(unsafe.Pointer(&macros[0])), 0,
		uintptr(unsafe.Pointer(entryC)), uintptr(unsafe.Pointer(targetC)),
		uintptr(flags), 0, uintptr(unsafe.Pointer(&code)), uintptr(unsafe.Pointer(&errs)))
	codeB, errB := com.FromRaw(code), com.FromRaw(errs)
	defer codeB.Release()
	defer errB.Release()
	var msg string
	if !errB.Nil() {
		msg = strings.TrimRight(string(blobBytes(errB)), "\x00\r\n ")
	}
	if com.HR(r).Failed() {
		return nil, fmt.Errorf("компиляция %s (%s, %s) не удалась:\n%s", name, entry, target, msg)
	}
	if msg != "" {
		l.Warn("шейдер %s: предупреждения:\n%s", name, msg)
	}
	return blobBytes(codeB), nil
}

func (l *ShaderLib) create(stage ShaderStage, code []byte) (com.Ptr, error) {
	idx := devCreateComputeShader
	switch stage {
	case StageVertex:
		idx = devCreateVertexShader
	case StagePixel:
		idx = devCreatePixelShader
	}
	var obj unsafe.Pointer
	r := l.dev.Dev.Call(idx, uintptr(unsafe.Pointer(&code[0])), uintptr(len(code)), 0, uintptr(unsafe.Pointer(&obj)))
	if err := com.Check(r, "Create*Shader"); err != nil {
		return com.Ptr{}, err
	}
	return com.FromRaw(obj), nil
}
