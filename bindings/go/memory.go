package fulgur

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// alloc invokes the wasm `fulgur_alloc(len)` export and returns the pointer.
// Returns 0 if the wasm allocator returned 0.
func alloc(ctx context.Context, mod api.Module, n uint32) (uint32, error) {
	fn := mod.ExportedFunction("fulgur_alloc")
	res, err := fn.Call(ctx, uint64(n))
	if err != nil {
		return 0, fmt.Errorf("fulgur_alloc(%d): %w", n, err)
	}
	return uint32(res[0]), nil
}

// free invokes `fulgur_free(ptr, len)`. Always paired with a prior alloc.
func free(ctx context.Context, mod api.Module, ptr, n uint32) error {
	fn := mod.ExportedFunction("fulgur_free")
	if _, err := fn.Call(ctx, uint64(ptr), uint64(n)); err != nil {
		return fmt.Errorf("fulgur_free(%d, %d): %w", ptr, n, err)
	}
	return nil
}

// writeBytes allocates inside wasm memory, copies `data` into it, and
// returns the (ptr, len). Caller must free with `free(ctx, mod, ptr, len)`
// when done.
func writeBytes(ctx context.Context, mod api.Module, data []byte) (uint32, uint32, error) {
	if len(data) == 0 {
		return 0, 0, nil
	}
	ptr, err := alloc(ctx, mod, uint32(len(data)))
	if err != nil {
		return 0, 0, err
	}
	if ptr == 0 {
		return 0, 0, fmt.Errorf("fulgur_alloc returned null for %d bytes", len(data))
	}
	if !mod.Memory().Write(ptr, data) {
		_ = free(ctx, mod, ptr, uint32(len(data)))
		return 0, 0, fmt.Errorf("memory.Write(%d, %d bytes) failed", ptr, len(data))
	}
	return ptr, uint32(len(data)), nil
}

// readBytes copies `n` bytes starting at `ptr` out of wasm memory.
func readBytes(mod api.Module, ptr, n uint32) ([]byte, error) {
	if n == 0 {
		return nil, nil
	}
	buf, ok := mod.Memory().Read(ptr, n)
	if !ok {
		return nil, fmt.Errorf("memory.Read(%d, %d) out of range", ptr, n)
	}
	out := make([]byte, n)
	copy(out, buf)
	return out, nil
}
