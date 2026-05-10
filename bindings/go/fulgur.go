// Package fulgur renders HTML/CSS to PDF by running the embedded
// fulgur-wasi WebAssembly module under wazero. See README.md for usage.
package fulgur

import (
	"context"
	_ "embed"
	"encoding/json"
)

//go:embed internal/wasm/fulgur.wasm
var wasmBytes []byte

// WASMSize returns the size in bytes of the embedded wasm module. Useful
// for diagnostics and for tests that want to confirm the embed worked.
func WASMSize() int { return len(wasmBytes) }

// Render renders the given HTML to a PDF. Goroutine-safe.
//
// If opts is non-nil, only the fields you set are applied — prior
// instance options for unspecified fields persist (partial sticky
// override). If opts is nil, the prior options are reused.
func (r *Renderer) Render(ctx context.Context, html []byte, opts *Options) ([]byte, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, &Error{Op: "Render", Message: "renderer is closed"}
	}
	r.inflight.Add(1)
	r.mu.Unlock()
	defer r.inflight.Done()

	inst, err := r.acquire(ctx)
	if err != nil {
		return nil, err
	}
	pdf, err := r.renderOnInstance(ctx, inst, html, opts)
	if err != nil {
		_ = inst.close(ctx)
		r.release(ctx, nil)
		return nil, err
	}
	r.release(ctx, inst)
	return pdf, nil
}

func (r *Renderer) renderOnInstance(
	ctx context.Context,
	inst *instance,
	html []byte,
	opts *Options,
) ([]byte, error) {
	if opts != nil {
		jsonBytes, err := json.Marshal(opts)
		if err != nil {
			return nil, err
		}
		if err := inst.configure(ctx, jsonBytes); err != nil {
			return nil, err
		}
	}
	return inst.render(ctx, html)
}
