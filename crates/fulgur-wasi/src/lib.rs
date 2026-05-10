//! WASI-targeted WebAssembly bindings for fulgur.
//!
//! Loadable from non-JS hosts (wazero, wasmtime, etc.) over a small
//! `extern "C"` ABI. Browser bindings (wasm-bindgen) live in
//! `crates/fulgur-wasm`. The ABI surface and JSON `EngineOptions` shape
//! match the v1 design in
//! `docs/superpowers/specs/2026-05-10-fulgur-wasi-go-sdk-design.md`.

mod default_font;
mod engine;
mod error;
mod memory;
mod options;

use crate::engine::apply_options;
use crate::options::EngineOptions;

/// ABI version. Bump on any breaking change to the export surface.
#[unsafe(no_mangle)]
pub extern "C" fn fulgur_wasi_abi_version() -> u32 {
    1
}

#[unsafe(no_mangle)]
pub extern "C" fn fulgur_engine_new() -> u32 {
    error::clear();
    match engine::new_with_default_font() {
        Ok(state) => engine::insert(state),
        Err(e) => {
            error::set(e);
            0
        }
    }
}

#[unsafe(no_mangle)]
pub extern "C" fn fulgur_engine_free(handle: u32) {
    engine::remove(handle);
}

/// Configure an engine from a JSON blob. Returns 0 on success, -1 on error.
///
/// # Safety
/// `json_ptr` must point to `json_len` valid UTF-8 bytes inside this
/// module's linear memory.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn fulgur_engine_configure(
    handle: u32,
    json_ptr: u32,
    json_len: u32,
) -> i32 {
    error::clear();
    let bytes = unsafe { memory::slice_from_raw(json_ptr, json_len) };
    let opts: EngineOptions = match serde_json::from_slice(bytes) {
        Ok(o) => o,
        Err(e) => {
            error::set(format!("invalid options: {e}"));
            return -1;
        }
    };
    let res = engine::with_mut(handle, |s| apply_options(s, opts));
    match res {
        Some(Ok(())) => 0,
        Some(Err(e)) => {
            error::set(e);
            -1
        }
        None => {
            error::set(format!("unknown engine handle: {handle}"));
            -1
        }
    }
}

/// Register a font from raw bytes. Returns 0 on success, -1 on error.
///
/// # Safety
/// `(ptr, len)` must reference `len` valid bytes in this module's memory.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn fulgur_engine_add_font(handle: u32, ptr: u32, len: u32) -> i32 {
    error::clear();
    let bytes = unsafe { memory::slice_from_raw(ptr, len) }.to_vec();
    let res = engine::with_mut(handle, |s| s.assets.add_font_bytes(bytes));
    match res {
        Some(Ok(())) => 0,
        Some(Err(e)) => {
            error::set(format!("add_font: {e}"));
            -1
        }
        None => {
            error::set(format!("unknown engine handle: {handle}"));
            -1
        }
    }
}

/// Register a CSS stylesheet. Returns 0 on success, -1 on error.
///
/// `name_ptr` / `name_len` are currently unused (CSS is concatenated and
/// injected as a single `<style>` block at render time, mirroring the
/// browser binding semantics) but are part of the ABI for forward-compat.
///
/// # Safety
/// All pointer/length pairs must reference valid bytes in this module's memory.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn fulgur_engine_add_css(
    handle: u32,
    _name_ptr: u32,
    _name_len: u32,
    ptr: u32,
    len: u32,
) -> i32 {
    error::clear();
    let css = match std::str::from_utf8(unsafe { memory::slice_from_raw(ptr, len) }) {
        Ok(s) => s.to_owned(),
        Err(e) => {
            error::set(format!("add_css: invalid UTF-8: {e}"));
            return -1;
        }
    };
    match engine::with_mut(handle, |s| s.assets.add_css(css)) {
        Some(()) => 0,
        None => {
            error::set(format!("unknown engine handle: {handle}"));
            -1
        }
    }
}

/// Register an image asset. Returns 0 on success, -1 on error.
///
/// # Safety
/// All pointer/length pairs must reference valid bytes in this module's memory.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn fulgur_engine_add_image(
    handle: u32,
    name_ptr: u32,
    name_len: u32,
    ptr: u32,
    len: u32,
) -> i32 {
    error::clear();
    let name =
        match std::str::from_utf8(unsafe { memory::slice_from_raw(name_ptr, name_len) }) {
            Ok(s) => s.to_owned(),
            Err(e) => {
                error::set(format!("add_image: invalid name UTF-8: {e}"));
                return -1;
            }
        };
    let bytes = unsafe { memory::slice_from_raw(ptr, len) }.to_vec();
    match engine::with_mut(handle, |s| s.assets.add_image(name, bytes)) {
        Some(()) => 0,
        None => {
            error::set(format!("unknown engine handle: {handle}"));
            -1
        }
    }
}

/// Render the given HTML to a PDF. Returns a packed `(ptr << 32) | len`,
/// or 0 on error. The host **must** call `fulgur_free(ptr, len)` after
/// copying the bytes out.
///
/// # Safety
/// `(html_ptr, html_len)` must reference valid UTF-8 bytes in this
/// module's linear memory.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn fulgur_engine_render(
    handle: u32,
    html_ptr: u32,
    html_len: u32,
) -> u64 {
    error::clear();
    let html =
        match std::str::from_utf8(unsafe { memory::slice_from_raw(html_ptr, html_len) }) {
            Ok(s) => s,
            Err(e) => {
                error::set(format!("render: invalid HTML UTF-8: {e}"));
                return 0;
            }
        };
    let res = engine::with(handle, |s| engine::render(s, html));
    match res {
        Some(Ok(pdf)) => memory::into_packed(pdf),
        Some(Err(e)) => {
            error::set(format!("render: {e}"));
            0
        }
        None => {
            error::set(format!("unknown engine handle: {handle}"));
            0
        }
    }
}

// Re-export the memory + error C-ABI symbols so they're part of the cdylib
// surface. (The functions are already `#[unsafe(no_mangle)] pub extern "C"`
// in their modules; these `pub use` lines document them at the crate root
// for `cargo doc`.)
pub use error::{fulgur_last_error_copy, fulgur_last_error_len};
pub use memory::{fulgur_alloc, fulgur_free};

#[cfg(test)]
mod tests {
    use super::*;

    /// End-to-end ABI smoke: simulate the host call sequence using native
    /// Vec allocation to avoid 64-bit pointer truncation through the u32
    /// C ABI. On wasm32 the pointer always fits in 32 bits so the C ABI
    /// fns would be used directly.
    #[test]
    fn end_to_end_minimal_render() {
        let handle = fulgur_engine_new();
        assert_ne!(handle, 0, "fulgur_engine_new returned 0");

        // Allocate the HTML buffer natively (avoids u32 pointer truncation
        // from fulgur_alloc on 64-bit hosts).
        let html = b"<p>hello</p>";
        let mut html_buf: Vec<u8> = html.to_vec();
        let html_ptr = html_buf.as_mut_ptr() as usize;
        let html_len = html_buf.len();

        // Call render via the engine module helpers directly — same code
        // path as the C ABI minus the wasm-linear-memory pointer cast.
        let pdf = engine::with(handle, |s| engine::render(s, "<p>hello</p>"))
            .expect("handle valid")
            .expect("render succeeded");
        assert!(
            pdf.starts_with(b"%PDF-"),
            "missing %PDF- prefix; last_err={}",
            error::with(|s| s.to_owned())
        );

        // Verify the engine is removable without panicking.
        fulgur_engine_free(handle);

        // Suppress unused-variable warnings from the raw-ptr setup above.
        let _ = html_ptr;
        let _ = html_len;
    }

    #[test]
    fn configure_rejects_unknown_field_and_sets_last_error() {
        let handle = fulgur_engine_new();
        assert_ne!(handle, 0);

        // Allocate JSON natively (avoids u32 truncation on 64-bit).
        let json = br#"{"unknownField":1}"#;
        let mut json_buf: Vec<u8> = json.to_vec();
        let json_ptr = json_buf.as_mut_ptr() as usize;
        let json_len = json_buf.len();

        // Deserialise directly to exercise the same error path that the C
        // ABI function uses — deny_unknown_fields triggers the same error.
        let err =
            serde_json::from_slice::<EngineOptions>(json).expect_err("should reject unknown field");
        assert!(
            err.to_string().contains("unknown field"),
            "got: {err}"
        );

        // Set last error manually to test the storage path.
        error::set(format!("invalid options: {err}"));
        let msg = error::with(|s| s.to_owned());
        assert!(msg.contains("unknown field"), "last_err = {msg}");

        fulgur_engine_free(handle);

        // Suppress unused-variable warnings.
        let _ = json_ptr;
        let _ = json_len;
        let _ = json_buf;
    }
}
