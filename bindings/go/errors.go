package fulgur

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// Error is the error type returned by Renderer.Render and friends. The
// Message field carries the wasm-side last_error string.
type Error struct {
	Op      string
	Message string
}

func (e *Error) Error() string {
	if e.Op == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Op, e.Message)
}

// fetchLastError reads the wasm-side last error message and wraps it.
// Returns a generic error if the message is empty.
func fetchLastError(ctx context.Context, mod api.Module, op string) error {
	lenFn := mod.ExportedFunction("fulgur_last_error_len")
	res, err := lenFn.Call(ctx)
	if err != nil {
		return fmt.Errorf("%s: fulgur_last_error_len: %w", op, err)
	}
	n := uint32(res[0])
	if n == 0 {
		return &Error{Op: op, Message: "(no last_error message)"}
	}
	ptr, err := alloc(ctx, mod, n)
	if err != nil {
		return fmt.Errorf("%s: alloc for last_error: %w", op, err)
	}
	defer func() { _ = free(ctx, mod, ptr, n) }()
	copyFn := mod.ExportedFunction("fulgur_last_error_copy")
	if _, err := copyFn.Call(ctx, uint64(ptr), uint64(n)); err != nil {
		return fmt.Errorf("%s: fulgur_last_error_copy: %w", op, err)
	}
	buf, err := readBytes(mod, ptr, n)
	if err != nil {
		return fmt.Errorf("%s: readBytes for last_error: %w", op, err)
	}
	return &Error{Op: op, Message: string(buf)}
}
