package fulgur

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// newTestRuntime returns a wazero runtime configured to use the interpreter
// backend. The wazevo arm64 JIT compiler panics on large wasm modules (BUG in
// resolveAddressingMode), so all tests use the interpreter instead.
func newTestRuntime(ctx context.Context) wazero.Runtime {
	return wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter())
}

func TestSmoke_LoadsAndCallsAbiVersion(t *testing.T) {
	if WASMSize() == 0 {
		t.Fatal("embedded wasm is empty; did the build run?")
	}

	ctx := context.Background()
	r := newTestRuntime(ctx)
	defer r.Close(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	mod, err := r.Instantiate(ctx, wasmBytes)
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}

	fn := mod.ExportedFunction("fulgur_wasi_abi_version")
	if fn == nil {
		t.Fatal("export fulgur_wasi_abi_version not found")
	}
	res, err := fn.Call(ctx)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if got := res[0]; got != 1 {
		t.Fatalf("abi_version = %d, want 1", got)
	}
}

func TestMemory_RoundtripBytes(t *testing.T) {
	ctx := context.Background()
	r := newTestRuntime(ctx)
	defer r.Close(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	mod, err := r.Instantiate(ctx, wasmBytes)
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}

	want := []byte("hello, fulgur")
	ptr, n, err := writeBytes(ctx, mod, want)
	if err != nil {
		t.Fatalf("writeBytes: %v", err)
	}
	got, err := readBytes(mod, ptr, n)
	if err != nil {
		t.Fatalf("readBytes: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("round-trip mismatch: got %q, want %q", got, want)
	}
	if err := free(ctx, mod, ptr, n); err != nil {
		t.Fatalf("free: %v", err)
	}
}

func TestErrors_FetchLastErrorAfterBadConfigure(t *testing.T) {
	ctx := context.Background()
	r := newTestRuntime(ctx)
	defer r.Close(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	mod, err := r.Instantiate(ctx, wasmBytes)
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}

	res, err := mod.ExportedFunction("fulgur_engine_new").Call(ctx)
	if err != nil {
		t.Fatalf("engine_new: %v", err)
	}
	handle := uint32(res[0])
	if handle == 0 {
		t.Fatal("engine_new returned 0")
	}

	json := []byte(`{"unknownField":1}`)
	ptr, n, err := writeBytes(ctx, mod, json)
	if err != nil {
		t.Fatalf("writeBytes: %v", err)
	}
	defer func() { _ = free(ctx, mod, ptr, n) }()

	res, err = mod.ExportedFunction("fulgur_engine_configure").Call(
		ctx, uint64(handle), uint64(ptr), uint64(n),
	)
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	rc := int32(res[0])
	if rc != -1 {
		t.Fatalf("configure rc = %d, want -1", rc)
	}

	gotErr := fetchLastError(ctx, mod, "configure")
	var fe *Error
	if !errorsAs(gotErr, &fe) {
		t.Fatalf("want *fulgur.Error, got %T (%v)", gotErr, gotErr)
	}
	if !contains(fe.Message, "unknown field") {
		t.Fatalf("want 'unknown field' in message, got %q", fe.Message)
	}
}

// errorsAs is a tiny helper to extract our typed *Error from an error
// chain without importing the stdlib `errors` package (which would
// shadow the local errors.go file).
func errorsAs(err error, target any) bool {
	type asErr interface{ As(any) bool }
	if x, ok := err.(asErr); ok {
		return x.As(target)
	}
	if pe, ok := err.(*Error); ok {
		if pp, ok := target.(**Error); ok {
			*pp = pe
			return true
		}
	}
	return false
}

func TestOptions_MarshalNamedPageSize(t *testing.T) {
	ps := PageSize{Name: "A4"}
	got, err := json.Marshal(Options{PageSize: &ps})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != `{"pageSize":"A4"}` {
		t.Fatalf("got %s", got)
	}
}

func TestOptions_MarshalCustomPageSize(t *testing.T) {
	ps := PageSize{WidthMM: 100, HeightMM: 150}
	got, err := json.Marshal(Options{PageSize: &ps})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != `{"pageSize":{"widthMm":100,"heightMm":150}}` {
		t.Fatalf("got %s", got)
	}
}

func TestOptions_MarshalMarginPt(t *testing.T) {
	pt := float32(36)
	got, err := json.Marshal(Options{Margin: &Margin{UniformPT: &pt}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != `{"margin":{"pt":36}}` {
		t.Fatalf("got %s", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
