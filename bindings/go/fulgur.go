// Package fulgur renders HTML/CSS to PDF by running the embedded
// fulgur-wasi WebAssembly module under wazero. See README.md for usage.
package fulgur

import _ "embed"

//go:embed internal/wasm/fulgur.wasm
var wasmBytes []byte

// WASMSize returns the size in bytes of the embedded wasm module. Useful
// for diagnostics and for tests that want to confirm the embed worked.
func WASMSize() int { return len(wasmBytes) }
