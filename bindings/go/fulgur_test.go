package fulgur

import (
	"context"
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
