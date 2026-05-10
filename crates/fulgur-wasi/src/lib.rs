//! WASI-targeted WebAssembly bindings for fulgur.
//!
//! Loadable from non-JS hosts (wazero, wasmtime, etc.) over a small
//! `extern "C"` ABI. Browser bindings (wasm-bindgen) live in
//! `crates/fulgur-wasm`.

mod error;
mod memory;

#[unsafe(no_mangle)]
pub extern "C" fn fulgur_wasi_abi_version() -> u32 {
    1
}
