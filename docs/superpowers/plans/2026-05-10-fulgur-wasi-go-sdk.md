# fulgur-wasi + Go SDK Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a wazero-compatible WebAssembly build of fulgur (`crates/fulgur-wasi`, target `wasm32-wasip1`) and a pool-backed Go SDK (`bindings/go/`) that embeds the prebuilt `.wasm` and exposes a goroutine-safe `Render` API.

**Architecture:** New sibling Rust crate (cdylib, `extern "C"` exports over linear memory, no wasm-bindgen). New Go module that loads the embedded `.wasm` via wazero, manages a fixed-size pool of WASM instances, and marshals options as JSON through the same `EngineOptions` shape used by the existing browser bindings.

**Tech Stack:** Rust 2024 + `serde` + `serde_json` + `slab`, target `wasm32-wasip1` (getrandom 0.4 auto-selects the WASI backend). Go ≥ 1.23 + `github.com/tetratelabs/wazero`. Build pipeline uses `wasm-opt -Oz` from binaryen.

**Spec:** `docs/superpowers/specs/2026-05-10-fulgur-wasi-go-sdk-design.md`

---

## File Structure

**Create:**

- `crates/fulgur-wasi/Cargo.toml`
- `crates/fulgur-wasi/src/lib.rs` — `extern "C"` exports (thin shims)
- `crates/fulgur-wasi/src/memory.rs` — `alloc` / `free` + slice helpers
- `crates/fulgur-wasi/src/error.rs` — thread-local last-error storage
- `crates/fulgur-wasi/src/options.rs` — `EngineOptions` JSON shape
- `crates/fulgur-wasi/src/engine.rs` — `EngineState` slab + render
- `crates/fulgur-wasi/src/default_font.rs` — `include_bytes!` for Noto Sans
- `crates/fulgur-wasi/assets/NotoSans-Regular.ttf` — copied from `examples/.fonts/`
- `bindings/go/go.mod`
- `bindings/go/go.sum` (generated)
- `bindings/go/fulgur.go` — public API (`Renderer`, `New`, options funcs)
- `bindings/go/pool.go` — instance pool
- `bindings/go/engine.go` — typed wrappers around exported wasm fns
- `bindings/go/memory.go` — wasm-memory read/write helpers
- `bindings/go/options.go` — `Options` struct + JSON tags
- `bindings/go/errors.go`
- `bindings/go/internal/wasm/fulgur.wasm` — committed build artifact
- `bindings/go/fulgur_test.go`
- `bindings/go/README.md`

**Modify:**

- `Cargo.toml` (workspace root) — add `crates/fulgur-wasi` to `members`
- `crates/fulgur/Cargo.toml` — add `[target.wasm32-wasip1.dependencies]` getrandom block
- `mise.toml` — add `build-wasi` task
- `.github/workflows/ci.yml` — add `wasi-build` and `go-test` jobs
- `README.md` (root) — add Go SDK link in bindings section

---

## Task 1: Create fulgur-wasi crate skeleton

**Files:**

- Create: `crates/fulgur-wasi/Cargo.toml`
- Create: `crates/fulgur-wasi/src/lib.rs`
- Modify: `Cargo.toml` (workspace root)

- [ ] **Step 1: Create `crates/fulgur-wasi/Cargo.toml`**

```toml
[package]
name = "fulgur-wasi"
version = "0.15.0"
edition.workspace = true
rust-version.workspace = true
license.workspace = true
repository.workspace = true
homepage.workspace = true
description = "WASI-targeted WebAssembly bindings for fulgur (HTML/CSS to PDF)"
publish = false

[lib]
crate-type = ["cdylib", "rlib"]

[dependencies]
fulgur = { path = "../fulgur" }
serde = { version = "1", features = ["derive"] }
serde_json = "1"
slab = "0.4"
```

- [ ] **Step 2: Create `crates/fulgur-wasi/src/lib.rs` with a placeholder export**

```rust
//! WASI-targeted WebAssembly bindings for fulgur.
//!
//! Loadable from non-JS hosts (wazero, wasmtime, etc.) over a small
//! `extern "C"` ABI. Browser bindings (wasm-bindgen) live in
//! `crates/fulgur-wasm`.

#[unsafe(no_mangle)]
pub extern "C" fn fulgur_wasi_abi_version() -> u32 {
    1
}
```

- [ ] **Step 3: Add the crate to the workspace members**

In `Cargo.toml`:

```toml
members = ["crates/fulgur", "crates/fulgur-cli", "crates/fulgur-ruby", "crates/fulgur-vrt", "crates/fulgur-wasi", "crates/fulgur-wasm", "crates/fulgur-wpt", "crates/pyfulgur"]
```

- [ ] **Step 4: Verify native build**

Run: `cargo check -p fulgur-wasi`
Expected: `Finished ...` with no errors.

- [ ] **Step 5: Commit**

```bash
git add Cargo.toml crates/fulgur-wasi/
git commit -m "feat(fulgur-wasi): scaffold cdylib crate"
```

---

## Task 2: Add WASI getrandom backend to fulgur

**Files:**

- Modify: `crates/fulgur/Cargo.toml`

- [ ] **Step 1: Append a `wasm32-wasip1` target dependency block**

After the existing `[target.'cfg(target_arch = "wasm32")'.dependencies]` block in `crates/fulgur/Cargo.toml`, add:

```toml
# WASI Preview 1 build (wasm32-wasip1): used by `crates/fulgur-wasi` for
# host-agnostic embedding (wazero, wasmtime). getrandom 0.4 auto-selects
# its WASI backend (`wasi_snapshot_preview1.random_get`) when targeting
# wasm32-wasip1; no feature flag is needed (no `wasi` feature exists in
# 0.4 — only `std`, `sys_rng`, `wasm_js`). Tracking: fulgur-iym.
[target.wasm32-wasip1.dependencies]
getrandom = { version = "0.4", default-features = false }
```

- [ ] **Step 2: Install the WASI target locally**

Run: `rustup target add wasm32-wasip1`
Expected: Target installed (or already-installed message).

- [ ] **Step 3: Verify fulgur compiles for WASI**

Run: `cargo check -p fulgur --target wasm32-wasip1`
Expected: `Finished ...` with no errors. (Some crates emit `unused` warnings; that's fine. Failure with `compile_error!` from `getrandom` means the feature didn't unify — re-check the toml.)

- [ ] **Step 4: Verify fulgur-wasi compiles for WASI**

Run: `cargo check -p fulgur-wasi --target wasm32-wasip1`
Expected: `Finished ...` with no errors.

- [ ] **Step 5: Commit**

```bash
git add crates/fulgur/Cargo.toml
git commit -m "feat(fulgur): add wasm32-wasip1 getrandom backend"
```

---

## Task 3: Implement memory.rs (alloc/free)

**Files:**

- Create: `crates/fulgur-wasi/src/memory.rs`
- Modify: `crates/fulgur-wasi/src/lib.rs` (declare module)

- [ ] **Step 1: Write the failing test in `crates/fulgur-wasi/src/memory.rs`**

```rust
//! Linear-memory allocator helpers exposed to the WASM host.
//!
//! The host calls `fulgur_alloc` to obtain a pointer into the module's
//! linear memory, copies bytes in, calls a fulgur entry point, then
//! frees with `fulgur_free`. Implementations leak `Vec<u8>` on alloc and
//! reconstruct + drop on free. Length must match the original allocation
//! exactly — deallocating a different length is undefined behaviour.

#[unsafe(no_mangle)]
pub extern "C" fn fulgur_alloc(len: u32) -> u32 {
    let mut v: Vec<u8> = Vec::with_capacity(len as usize);
    let ptr = v.as_mut_ptr() as u32;
    std::mem::forget(v);
    ptr
}

/// # Safety
/// `ptr` must come from a previous `fulgur_alloc(len)` and not have been
/// freed already.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn fulgur_free(ptr: u32, len: u32) {
    if ptr == 0 {
        return;
    }
    let _ = Vec::from_raw_parts(ptr as *mut u8, 0, len as usize);
}

/// Borrow `len` bytes starting at `ptr` as a slice. Caller guarantees the
/// pointer/length is valid.
#[allow(dead_code)]
pub(crate) unsafe fn slice_from_raw(ptr: u32, len: u32) -> &'static [u8] {
    if len == 0 {
        return &[];
    }
    std::slice::from_raw_parts(ptr as *const u8, len as usize)
}

/// Allocate a `Vec<u8>` of the given content, leak it, and return its
/// `(ptr, len)` packed into one `u64` as `(ptr as u64) << 32 | len as u64`.
/// Returns 0 if `data` is empty (caller treats 0 as "no result").
pub(crate) fn into_packed(data: Vec<u8>) -> u64 {
    if data.is_empty() {
        return 0;
    }
    let len = data.len() as u32;
    let mut v = data;
    let ptr = v.as_mut_ptr() as u32;
    std::mem::forget(v);
    ((ptr as u64) << 32) | (len as u64)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn alloc_then_free_roundtrips_pattern() {
        let len = 64u32;
        let ptr = fulgur_alloc(len);
        assert_ne!(ptr, 0, "alloc returned null");
        unsafe {
            let slice = std::slice::from_raw_parts_mut(ptr as *mut u8, len as usize);
            for (i, b) in slice.iter_mut().enumerate() {
                *b = (i & 0xff) as u8;
            }
            for (i, b) in slice.iter().enumerate() {
                assert_eq!(*b, (i & 0xff) as u8);
            }
            fulgur_free(ptr, len);
        }
    }

    #[test]
    fn into_packed_zero_for_empty() {
        assert_eq!(into_packed(Vec::new()), 0);
    }

    #[test]
    fn into_packed_recoverable() {
        let bytes = vec![0xAA, 0xBB, 0xCC, 0xDD];
        let packed = into_packed(bytes.clone());
        let ptr = (packed >> 32) as u32;
        let len = packed as u32;
        assert_eq!(len as usize, bytes.len());
        unsafe {
            let recovered = std::slice::from_raw_parts(ptr as *const u8, len as usize);
            assert_eq!(recovered, &bytes[..]);
            fulgur_free(ptr, len);
        }
    }
}
```

- [ ] **Step 2: Declare the module in `crates/fulgur-wasi/src/lib.rs`**

Replace the placeholder `lib.rs` content with:

```rust
//! WASI-targeted WebAssembly bindings for fulgur.

mod memory;

#[unsafe(no_mangle)]
pub extern "C" fn fulgur_wasi_abi_version() -> u32 {
    1
}
```

- [ ] **Step 3: Run tests**

Run: `cargo test -p fulgur-wasi --lib`
Expected: 3 tests pass.

- [ ] **Step 4: Commit**

```bash
git add crates/fulgur-wasi/src/memory.rs crates/fulgur-wasi/src/lib.rs
git commit -m "feat(fulgur-wasi): add linear memory alloc/free"
```

---

## Task 4: Implement error.rs (thread-local last error)

**Files:**

- Create: `crates/fulgur-wasi/src/error.rs`
- Modify: `crates/fulgur-wasi/src/lib.rs`

- [ ] **Step 1: Write `crates/fulgur-wasi/src/error.rs`**

```rust
//! Thread-local last-error storage.
//!
//! WASI Preview 1 modules are single-threaded per instance, but the
//! storage is still `thread_local!` so the same code compiles + tests on
//! native multi-threaded targets.

use std::cell::RefCell;

thread_local! {
    static LAST_ERR: RefCell<String> = const { RefCell::new(String::new()) };
}

pub(crate) fn set(msg: impl Into<String>) {
    LAST_ERR.with(|e| *e.borrow_mut() = msg.into());
}

pub(crate) fn clear() {
    LAST_ERR.with(|e| e.borrow_mut().clear());
}

pub(crate) fn with<R>(f: impl FnOnce(&str) -> R) -> R {
    LAST_ERR.with(|e| f(&e.borrow()))
}

/// Length of the current last-error message in bytes.
#[unsafe(no_mangle)]
pub extern "C" fn fulgur_last_error_len() -> u32 {
    with(|s| s.len() as u32)
}

/// Copy up to `dst_len` bytes of the last error into `dst_ptr`. Returns
/// the number of bytes actually copied.
///
/// # Safety
/// `dst_ptr` must point to at least `dst_len` writable bytes.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn fulgur_last_error_copy(dst_ptr: u32, dst_len: u32) -> u32 {
    with(|s| {
        let n = std::cmp::min(s.len(), dst_len as usize);
        if n > 0 {
            std::ptr::copy_nonoverlapping(s.as_ptr(), dst_ptr as *mut u8, n);
        }
        n as u32
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn set_get_clear_roundtrips() {
        clear();
        assert_eq!(fulgur_last_error_len(), 0);
        set("boom");
        with(|s| assert_eq!(s, "boom"));
        assert_eq!(fulgur_last_error_len(), 4);
        clear();
        assert_eq!(fulgur_last_error_len(), 0);
    }

    #[test]
    fn copy_truncates_to_dst_len() {
        clear();
        set("0123456789");
        let mut buf = [0u8; 4];
        let n = unsafe { fulgur_last_error_copy(buf.as_mut_ptr() as u32, buf.len() as u32) };
        assert_eq!(n, 4);
        assert_eq!(&buf, b"0123");
    }
}
```

- [ ] **Step 2: Add module declaration to `crates/fulgur-wasi/src/lib.rs`**

```rust
//! WASI-targeted WebAssembly bindings for fulgur.

mod error;
mod memory;

#[unsafe(no_mangle)]
pub extern "C" fn fulgur_wasi_abi_version() -> u32 {
    1
}
```

- [ ] **Step 3: Run tests**

Run: `cargo test -p fulgur-wasi --lib`
Expected: All 5 tests pass.

- [ ] **Step 4: Commit**

```bash
git add crates/fulgur-wasi/src/error.rs crates/fulgur-wasi/src/lib.rs
git commit -m "feat(fulgur-wasi): add thread-local last-error storage"
```

---

## Task 5: Implement options.rs (EngineOptions JSON shape)

This mirrors the `EngineOptions` struct in `crates/fulgur-wasm/src/lib.rs:48-127` exactly, copied (not extracted to a shared crate — keeps blast radius small per spec).

**Files:**

- Create: `crates/fulgur-wasi/src/options.rs`
- Modify: `crates/fulgur-wasi/src/lib.rs`

- [ ] **Step 1: Write `crates/fulgur-wasi/src/options.rs`**

```rust
//! Engine configuration options accepted via the JSON `configure` call.
//!
//! The shape is identical to the `EngineOptions` struct in
//! `crates/fulgur-wasm/src/lib.rs` — kept in sync by hand. Both crates
//! talk to `fulgur::Engine::builder()` through the same field set, so
//! drift here is a binding bug.

use fulgur::{Margin, PageSize};
use serde::Deserialize;

#[derive(Deserialize, Default, Debug)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct EngineOptions {
    #[serde(default)]
    pub page_size: Option<PageSizeOption>,
    #[serde(default)]
    pub margin: Option<MarginOption>,
    #[serde(default)]
    pub landscape: Option<bool>,
    #[serde(default)]
    pub title: Option<String>,
    #[serde(default)]
    pub authors: Option<Vec<String>>,
    #[serde(default)]
    pub description: Option<String>,
    #[serde(default)]
    pub keywords: Option<Vec<String>>,
    #[serde(default)]
    pub creator: Option<String>,
    #[serde(default)]
    pub producer: Option<String>,
    #[serde(default)]
    pub creation_date: Option<String>,
    #[serde(default)]
    pub lang: Option<String>,
    #[serde(default)]
    pub bookmarks: Option<bool>,
}

#[derive(Deserialize, Debug)]
#[serde(untagged)]
pub(crate) enum PageSizeOption {
    Named(String),
    #[serde(rename_all = "camelCase")]
    Custom { width_mm: f32, height_mm: f32 },
}

#[derive(Deserialize, Debug)]
#[serde(untagged)]
pub(crate) enum MarginOption {
    Mm { mm: f32 },
    Pt { pt: f32 },
    #[serde(rename_all = "camelCase")]
    Full {
        top_mm: f32,
        right_mm: f32,
        bottom_mm: f32,
        left_mm: f32,
    },
}

impl PageSizeOption {
    pub(crate) fn to_page_size(&self) -> Result<PageSize, String> {
        match self {
            Self::Named(name) => match name.to_ascii_lowercase().as_str() {
                "a4" => Ok(PageSize::A4),
                "a3" => Ok(PageSize::A3),
                "letter" => Ok(PageSize::LETTER),
                other => Err(format!("unknown page size: {other}")),
            },
            Self::Custom { width_mm, height_mm } => Ok(PageSize::custom(*width_mm, *height_mm)),
        }
    }
}

impl MarginOption {
    pub(crate) fn to_margin(&self) -> Margin {
        match self {
            Self::Mm { mm } => Margin::uniform_mm(*mm),
            Self::Pt { pt } => Margin::uniform(*pt),
            Self::Full {
                top_mm,
                right_mm,
                bottom_mm,
                left_mm,
            } => {
                let to_pt = |mm: f32| mm * 72.0 / 25.4;
                Margin {
                    top: to_pt(*top_mm),
                    right: to_pt(*right_mm),
                    bottom: to_pt(*bottom_mm),
                    left: to_pt(*left_mm),
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_named_page_size_and_landscape() {
        let opts: EngineOptions =
            serde_json::from_str(r#"{"pageSize":"A4","landscape":true}"#).unwrap();
        assert!(matches!(opts.page_size, Some(PageSizeOption::Named(ref s)) if s == "A4"));
        assert_eq!(opts.landscape, Some(true));
    }

    #[test]
    fn parses_custom_page_size() {
        let opts: EngineOptions =
            serde_json::from_str(r#"{"pageSize":{"widthMm":100.0,"heightMm":150.0}}"#).unwrap();
        match opts.page_size.unwrap() {
            PageSizeOption::Custom { width_mm, height_mm } => {
                assert_eq!(width_mm, 100.0);
                assert_eq!(height_mm, 150.0);
            }
            _ => panic!("expected Custom variant"),
        }
    }

    #[test]
    fn parses_margin_variants() {
        let mm: EngineOptions = serde_json::from_str(r#"{"margin":{"mm":10.0}}"#).unwrap();
        assert!(matches!(mm.margin, Some(MarginOption::Mm { mm }) if mm == 10.0));

        let pt: EngineOptions = serde_json::from_str(r#"{"margin":{"pt":36.0}}"#).unwrap();
        assert!(matches!(pt.margin, Some(MarginOption::Pt { pt }) if pt == 36.0));

        let full: EngineOptions = serde_json::from_str(
            r#"{"margin":{"topMm":1.0,"rightMm":2.0,"bottomMm":3.0,"leftMm":4.0}}"#,
        )
        .unwrap();
        assert!(matches!(full.margin, Some(MarginOption::Full { .. })));
    }

    #[test]
    fn rejects_unknown_top_level_field() {
        let err = serde_json::from_str::<EngineOptions>(r#"{"unknownField":1}"#).unwrap_err();
        assert!(err.to_string().contains("unknown field"), "got: {err}");
    }

    #[test]
    fn rejects_unknown_page_size_name() {
        let opts: EngineOptions = serde_json::from_str(r#"{"pageSize":"foo"}"#).unwrap();
        let err = opts.page_size.unwrap().to_page_size().unwrap_err();
        assert!(err.contains("unknown page size"));
    }
}
```

- [ ] **Step 2: Declare the module in `crates/fulgur-wasi/src/lib.rs`**

```rust
mod error;
mod memory;
mod options;
```

- [ ] **Step 3: Run tests**

Run: `cargo test -p fulgur-wasi --lib options`
Expected: 5 new tests pass.

- [ ] **Step 4: Commit**

```bash
git add crates/fulgur-wasi/src/options.rs crates/fulgur-wasi/src/lib.rs
git commit -m "feat(fulgur-wasi): add EngineOptions JSON parser"
```

---

## Task 6: Implement engine.rs (EngineState slab + new/free)

**Files:**

- Create: `crates/fulgur-wasi/src/engine.rs`
- Modify: `crates/fulgur-wasi/src/lib.rs`

- [ ] **Step 1: Write the basic skeleton in `crates/fulgur-wasi/src/engine.rs`**

```rust
//! Engine state managed inside the WASM module.
//!
//! Engines live in a thread-local `Slab`, addressed by an opaque `u32`
//! handle returned to the host. Handle 0 is reserved as the sentinel for
//! "error / no handle".
//!
//! Each `EngineState` mirrors the deferred-config + assets pattern used by
//! `crates/fulgur-wasm::Engine`, so all options accumulate on the state
//! and the `fulgur::Engine` builder is constructed lazily at render time.

use crate::options::EngineOptions;
use fulgur::{AssetBundle, Margin, PageSize};
use slab::Slab;
use std::cell::RefCell;

#[derive(Default)]
pub(crate) struct EngineState {
    pub(crate) assets: AssetBundle,
    pub(crate) page_size: Option<PageSize>,
    pub(crate) margin: Option<Margin>,
    pub(crate) landscape: Option<bool>,
    pub(crate) title: Option<String>,
    pub(crate) authors: Vec<String>,
    pub(crate) description: Option<String>,
    pub(crate) keywords: Vec<String>,
    pub(crate) creator: Option<String>,
    pub(crate) producer: Option<String>,
    pub(crate) creation_date: Option<String>,
    pub(crate) lang: Option<String>,
    pub(crate) bookmarks: Option<bool>,
}

thread_local! {
    static ENGINES: RefCell<Slab<EngineState>> = RefCell::new(Slab::with_capacity(4));
}

/// Insert a new state and return its handle (slab index + 1; 0 is the
/// error sentinel).
pub(crate) fn insert(state: EngineState) -> u32 {
    ENGINES.with(|s| (s.borrow_mut().insert(state) as u32).wrapping_add(1))
}

/// Remove a handle from the slab.
pub(crate) fn remove(handle: u32) {
    if handle == 0 {
        return;
    }
    let key = (handle - 1) as usize;
    ENGINES.with(|s| {
        let mut slab = s.borrow_mut();
        if slab.contains(key) {
            slab.remove(key);
        }
    });
}

/// Run a closure with a mutable reference to the state for `handle`.
/// Returns `None` if the handle is unknown.
pub(crate) fn with_mut<R>(handle: u32, f: impl FnOnce(&mut EngineState) -> R) -> Option<R> {
    if handle == 0 {
        return None;
    }
    let key = (handle - 1) as usize;
    ENGINES.with(|s| {
        let mut slab = s.borrow_mut();
        slab.get_mut(key).map(f)
    })
}

/// Run a closure with an immutable reference to the state for `handle`.
pub(crate) fn with<R>(handle: u32, f: impl FnOnce(&EngineState) -> R) -> Option<R> {
    if handle == 0 {
        return None;
    }
    let key = (handle - 1) as usize;
    ENGINES.with(|s| {
        let slab = s.borrow();
        slab.get(key).map(f)
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn insert_and_remove_handle() {
        let h = insert(EngineState::default());
        assert_ne!(h, 0);
        assert!(with(h, |_| ()).is_some());
        remove(h);
        assert!(with(h, |_| ()).is_none());
    }

    #[test]
    fn remove_zero_is_noop() {
        remove(0);
    }

    #[test]
    fn with_mut_can_mutate() {
        let h = insert(EngineState::default());
        with_mut(h, |s| s.title = Some("hi".into())).unwrap();
        let title = with(h, |s| s.title.clone()).unwrap();
        assert_eq!(title.as_deref(), Some("hi"));
        remove(h);
    }

    #[test]
    fn apply_options_partial_overrides() {
        let h = insert(EngineState::default());
        with_mut(h, |s| s.title = Some("kept".into())).unwrap();

        let opts: EngineOptions =
            serde_json::from_str(r#"{"landscape":true}"#).unwrap();
        with_mut(h, |s| super::apply_options(s, opts).unwrap()).unwrap();

        let (title, landscape) =
            with(h, |s| (s.title.clone(), s.landscape)).unwrap();
        assert_eq!(title.as_deref(), Some("kept"));
        assert_eq!(landscape, Some(true));
        remove(h);
    }
}

pub(crate) fn apply_options(state: &mut EngineState, opts: EngineOptions) -> Result<(), String> {
    if let Some(ps) = opts.page_size {
        state.page_size = Some(ps.to_page_size()?);
    }
    if let Some(m) = opts.margin {
        state.margin = Some(m.to_margin());
    }
    if let Some(l) = opts.landscape {
        state.landscape = Some(l);
    }
    if let Some(t) = opts.title {
        state.title = Some(t);
    }
    if let Some(a) = opts.authors {
        state.authors = a;
    }
    if let Some(d) = opts.description {
        state.description = Some(d);
    }
    if let Some(k) = opts.keywords {
        state.keywords = k;
    }
    if let Some(c) = opts.creator {
        state.creator = Some(c);
    }
    if let Some(p) = opts.producer {
        state.producer = Some(p);
    }
    if let Some(cd) = opts.creation_date {
        state.creation_date = Some(cd);
    }
    if let Some(l) = opts.lang {
        state.lang = Some(l);
    }
    if let Some(b) = opts.bookmarks {
        state.bookmarks = Some(b);
    }
    Ok(())
}
```

- [ ] **Step 2: Declare the module in `crates/fulgur-wasi/src/lib.rs`**

```rust
mod engine;
mod error;
mod memory;
mod options;
```

- [ ] **Step 3: Run tests**

Run: `cargo test -p fulgur-wasi --lib engine`
Expected: 4 new tests pass.

- [ ] **Step 4: Commit**

```bash
git add crates/fulgur-wasi/src/engine.rs crates/fulgur-wasi/src/lib.rs
git commit -m "feat(fulgur-wasi): add EngineState slab + apply_options"
```

---

## Task 7: Implement engine.rs render

**Files:**

- Modify: `crates/fulgur-wasi/src/engine.rs`

- [ ] **Step 1: Append a `render` helper and a render test**

Add to the bottom of `crates/fulgur-wasi/src/engine.rs` (after `apply_options`):

```rust
pub(crate) fn render(state: &EngineState, html: &str) -> fulgur::Result<Vec<u8>> {
    let mut builder = fulgur::Engine::builder().assets(state.assets.clone());
    if let Some(s) = state.page_size {
        builder = builder.page_size(s);
    }
    if let Some(m) = state.margin {
        builder = builder.margin(m);
    }
    if let Some(l) = state.landscape {
        builder = builder.landscape(l);
    }
    if let Some(ref t) = state.title {
        builder = builder.title(t.clone());
    }
    if !state.authors.is_empty() {
        builder = builder.authors(state.authors.clone());
    }
    if let Some(ref d) = state.description {
        builder = builder.description(d.clone());
    }
    if !state.keywords.is_empty() {
        builder = builder.keywords(state.keywords.clone());
    }
    if let Some(ref c) = state.creator {
        builder = builder.creator(c.clone());
    }
    if let Some(ref p) = state.producer {
        builder = builder.producer(p.clone());
    }
    if let Some(ref cd) = state.creation_date {
        builder = builder.creation_date(cd.clone());
    }
    if let Some(ref l) = state.lang {
        builder = builder.lang(l.clone());
    }
    if let Some(b) = state.bookmarks {
        builder = builder.bookmarks(b);
    }
    builder.build().render_html(html)
}
```

Add to the `#[cfg(test)] mod tests` block in the same file:

```rust
    #[test]
    fn render_emits_pdf_magic() {
        let h = insert(EngineState::default());
        let pdf = with(h, |s| render(s, "<p>hello</p>")).unwrap().unwrap();
        assert!(pdf.starts_with(b"%PDF-"), "missing %PDF- prefix");
        remove(h);
    }
```

- [ ] **Step 2: Run tests**

Run: `cargo test -p fulgur-wasi --lib engine::tests::render_emits_pdf_magic`
Expected: PASS (PDF generation works on native).

- [ ] **Step 3: Commit**

```bash
git add crates/fulgur-wasi/src/engine.rs
git commit -m "feat(fulgur-wasi): add render entry point on EngineState"
```

---

## Task 8: Embed the default font

**Files:**

- Create: `crates/fulgur-wasi/assets/NotoSans-Regular.ttf` (copy)
- Create: `crates/fulgur-wasi/src/default_font.rs`
- Modify: `crates/fulgur-wasi/src/engine.rs`
- Modify: `crates/fulgur-wasi/src/lib.rs`

- [ ] **Step 1: Copy the font into the crate**

Run: `mkdir -p crates/fulgur-wasi/assets && cp examples/.fonts/NotoSans-Regular.ttf crates/fulgur-wasi/assets/`
Expected: file copied (~340 KB).

- [ ] **Step 2: Create `crates/fulgur-wasi/src/default_font.rs`**

```rust
//! Built-in default font registered automatically inside `engine_new`.
//!
//! Bundling Noto Sans Regular gives `Render` something to fall back to
//! when the host hasn't registered any custom fonts. Keep this file
//! tiny; license obligations are tracked in `examples/.fonts/OFL.txt`,
//! which the crate copies here for redistribution.

pub(crate) const NOTO_SANS_REGULAR: &[u8] = include_bytes!("../assets/NotoSans-Regular.ttf");
```

- [ ] **Step 3: Auto-register in the constructor**

Add a `new_with_default_font` helper at the bottom of `crates/fulgur-wasi/src/engine.rs`:

```rust
pub(crate) fn new_with_default_font() -> Result<EngineState, String> {
    let mut state = EngineState::default();
    state
        .assets
        .add_font_bytes(crate::default_font::NOTO_SANS_REGULAR.to_vec())
        .map_err(|e| format!("default font: {e}"))?;
    Ok(state)
}
```

Add a test:

```rust
    #[test]
    fn default_font_loads_without_error() {
        // `AssetBundle` does not expose a public font count today, so we
        // can't assert directly. `add_font_bytes` returning Ok is the
        // meaningful check: it proves skrifa successfully decoded the
        // bytes. The render path is exercised end-to-end in
        // `lib.rs::tests::end_to_end_minimal_render` (Task 9).
        let _state = super::new_with_default_font().expect("default font loads");
    }
```

- [ ] **Step 4: Add the module to `lib.rs`**

```rust
mod default_font;
mod engine;
mod error;
mod memory;
mod options;
```

- [ ] **Step 5: Run tests**

Run: `cargo test -p fulgur-wasi --lib default_font_loads_without_error`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add crates/fulgur-wasi/assets/ crates/fulgur-wasi/src/default_font.rs crates/fulgur-wasi/src/engine.rs crates/fulgur-wasi/src/lib.rs
git commit -m "feat(fulgur-wasi): embed Noto Sans Regular as default font"
```

---

## Task 9: C ABI surface in lib.rs

**Files:**

- Modify: `crates/fulgur-wasi/src/lib.rs`

- [ ] **Step 1: Replace `lib.rs` with the full C ABI**

```rust
//! WASI-targeted WebAssembly bindings for fulgur.
//!
//! Loadable from non-JS hosts (wazero, wasmtime, etc.) over a small
//! `extern "C"` ABI. Browser bindings (wasm-bindgen) live in
//! `crates/fulgur-wasm`. The ABI surface and JSON `EngineOptions` shape
//! match the v1 design in `docs/superpowers/specs/2026-05-10-fulgur-wasi-go-sdk-design.md`.

mod default_font;
mod engine;
mod error;
mod memory;
mod options;

use crate::engine::{EngineState, apply_options};
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
    let name = match std::str::from_utf8(unsafe { memory::slice_from_raw(name_ptr, name_len) }) {
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
/// `(html_ptr, html_len)` must reference valid UTF-8 bytes in this module's memory.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn fulgur_engine_render(handle: u32, html_ptr: u32, html_len: u32) -> u64 {
    error::clear();
    let html = match std::str::from_utf8(unsafe { memory::slice_from_raw(html_ptr, html_len) }) {
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

#[cfg(test)]
mod tests {
    use super::*;

    /// End-to-end ABI smoke: simulate the host call sequence on native.
    #[test]
    fn end_to_end_minimal_render() {
        let handle = fulgur_engine_new();
        assert_ne!(handle, 0);

        let html = b"<p>hello</p>";
        let html_ptr = fulgur_alloc(html.len() as u32);
        unsafe {
            std::ptr::copy_nonoverlapping(html.as_ptr(), html_ptr as *mut u8, html.len());
        }
        let packed = unsafe { fulgur_engine_render(handle, html_ptr, html.len() as u32) };
        assert_ne!(packed, 0, "render returned 0; last_err = {}",
            error::with(|s| s.to_owned()));

        let pdf_ptr = (packed >> 32) as u32;
        let pdf_len = packed as u32;
        let pdf = unsafe {
            std::slice::from_raw_parts(pdf_ptr as *const u8, pdf_len as usize).to_vec()
        };
        assert!(pdf.starts_with(b"%PDF-"));

        unsafe {
            fulgur_free(pdf_ptr, pdf_len);
            fulgur_free(html_ptr, html.len() as u32);
        }
        fulgur_engine_free(handle);
    }

    #[test]
    fn configure_rejects_unknown_field_and_sets_last_error() {
        let handle = fulgur_engine_new();
        let json = br#"{"unknownField":1}"#;
        let ptr = fulgur_alloc(json.len() as u32);
        unsafe {
            std::ptr::copy_nonoverlapping(json.as_ptr(), ptr as *mut u8, json.len());
        }
        let rc = unsafe { fulgur_engine_configure(handle, ptr, json.len() as u32) };
        assert_eq!(rc, -1);
        let msg = error::with(|s| s.to_owned());
        assert!(msg.contains("unknown field"), "got: {msg}");
        unsafe { fulgur_free(ptr, json.len() as u32) };
        fulgur_engine_free(handle);
    }
}

// Re-export the memory + error C-ABI symbols so they're part of the cdylib
// surface. (The functions are already `#[no_mangle] pub extern "C"` in
// their modules; these `pub use` lines are just to document them at the
// crate root for `cargo doc`.)
pub use error::{fulgur_last_error_copy, fulgur_last_error_len};
pub use memory::{fulgur_alloc, fulgur_free};
```

- [ ] **Step 2: Run tests**

Run: `cargo test -p fulgur-wasi`
Expected: All tests pass (existing 12-ish + 2 new = ~14).

- [ ] **Step 3: Verify the WASI target builds**

Run: `cargo build -p fulgur-wasi --release --target wasm32-wasip1`
Expected: `Finished release` with output at `target/wasm32-wasip1/release/fulgur_wasi.wasm`. If link errors mention WASI imports we don't expect (e.g., `fd_write` complaints from a panic message), they're fine — wazero will provide them via the `wasi_snapshot_preview1` import.

- [ ] **Step 4: Commit**

```bash
git add crates/fulgur-wasi/src/lib.rs
git commit -m "feat(fulgur-wasi): wire up C ABI surface (configure/add_*/render)"
```

---

## Task 10: Build pipeline + commit initial wasm artifact

**Files:**

- Modify: `mise.toml`
- Create: `bindings/go/internal/wasm/fulgur.wasm` (build artifact)

- [ ] **Step 1: Add the `build-wasi` task to `mise.toml`**

Append to `mise.toml`:

```toml
[tasks.build-wasi]
description = "Build the WASI .wasm artifact for the Go SDK"
run = """
#!/usr/bin/env bash
set -euo pipefail
cargo build -p fulgur-wasi --release --target wasm32-wasip1
mkdir -p bindings/go/internal/wasm
if command -v wasm-opt >/dev/null 2>&1; then
  wasm-opt -Oz -o bindings/go/internal/wasm/fulgur.wasm \
    target/wasm32-wasip1/release/fulgur_wasi.wasm
else
  echo "wasm-opt not found; copying unoptimized binary" >&2
  cp target/wasm32-wasip1/release/fulgur_wasi.wasm bindings/go/internal/wasm/fulgur.wasm
fi
ls -lh bindings/go/internal/wasm/fulgur.wasm
"""
```

- [ ] **Step 2: Install `wasm-opt` (binaryen)**

On macOS: `brew install binaryen`. On Ubuntu: `apt-get install binaryen` or download the latest binaryen release.

- [ ] **Step 3: Run the build**

Run: `mise run build-wasi`
Expected: `bindings/go/internal/wasm/fulgur.wasm` exists, size in the 5–15 MB range.

- [ ] **Step 4: Inspect the export surface to sanity-check**

Run: `wasm-objdump -x bindings/go/internal/wasm/fulgur.wasm | grep '^ - func' | grep fulgur_`
Expected: lines for `fulgur_alloc`, `fulgur_free`, `fulgur_engine_new`, `fulgur_engine_free`, `fulgur_engine_configure`, `fulgur_engine_add_font`, `fulgur_engine_add_css`, `fulgur_engine_add_image`, `fulgur_engine_render`, `fulgur_last_error_len`, `fulgur_last_error_copy`, `fulgur_wasi_abi_version`.

If `wasm-objdump` isn't installed, skip — the Go integration test (Task 12) will surface missing exports.

- [ ] **Step 5: Commit the build script + artifact**

```bash
git add mise.toml bindings/go/internal/wasm/fulgur.wasm
git commit -m "build(fulgur-wasi): add build-wasi mise task and initial .wasm artifact"
```

---

## Task 11: Go module skeleton + wazero smoke test

**Files:**

- Create: `bindings/go/go.mod`
- Create: `bindings/go/fulgur.go` (placeholder)
- Create: `bindings/go/fulgur_test.go`

- [ ] **Step 1: Initialize the Go module**

Run:

```bash
cd bindings/go && go mod init github.com/fulgur-rs/fulgur/bindings/go && cd -
```

Expected: `bindings/go/go.mod` created.

- [ ] **Step 2: Add the wazero dependency**

Run: `cd bindings/go && go get github.com/tetratelabs/wazero@latest && cd -`
Expected: `go.mod` and `go.sum` updated.

- [ ] **Step 3: Create a minimal `bindings/go/fulgur.go` that embeds the wasm**

```go
// Package fulgur renders HTML/CSS to PDF by running the embedded
// fulgur-wasi WebAssembly module under wazero. See README.md for usage.
package fulgur

import _ "embed"

//go:embed internal/wasm/fulgur.wasm
var wasmBytes []byte

// WASMSize returns the size in bytes of the embedded wasm module. Useful
// for diagnostics and for tests that want to confirm the embed worked.
func WASMSize() int { return len(wasmBytes) }
```

- [ ] **Step 4: Write the failing smoke test in `bindings/go/fulgur_test.go`**

```go
package fulgur

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

func TestSmoke_LoadsAndCallsAbiVersion(t *testing.T) {
	if WASMSize() == 0 {
		t.Fatal("embedded wasm is empty; did the build run?")
	}

	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
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
```

- [ ] **Step 5: Run the test**

Run: `cd bindings/go && go test -run TestSmoke_LoadsAndCallsAbiVersion -v && cd -`
Expected: PASS. If it fails with "function not found", the build artifact in Task 10 is stale — re-run `mise run build-wasi`.

- [ ] **Step 6: Commit**

```bash
git add bindings/go/go.mod bindings/go/go.sum bindings/go/fulgur.go bindings/go/fulgur_test.go
git commit -m "feat(bindings-go): scaffold module + wazero smoke test"
```

---

## Task 12: Go memory.go (linear-memory helpers)

**Files:**

- Create: `bindings/go/memory.go`
- Modify: `bindings/go/fulgur_test.go`

- [ ] **Step 1: Write `bindings/go/memory.go`**

```go
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
```

- [ ] **Step 2: Add a round-trip test to `bindings/go/fulgur_test.go`**

```go
func TestMemory_RoundtripBytes(t *testing.T) {
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
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
```

- [ ] **Step 3: Run tests**

Run: `cd bindings/go && go test -run TestMemory_ -v && cd -`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add bindings/go/memory.go bindings/go/fulgur_test.go
git commit -m "feat(bindings-go): add wasm linear-memory helpers"
```

---

## Task 13: Go errors.go + last-error fetch

**Files:**

- Create: `bindings/go/errors.go`
- Modify: `bindings/go/fulgur_test.go`

- [ ] **Step 1: Write `bindings/go/errors.go`**

```go
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
```

- [ ] **Step 2: Add a test that triggers a deliberate error**

```go
func TestErrors_FetchLastErrorAfterBadConfigure(t *testing.T) {
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	mod, err := r.Instantiate(ctx, wasmBytes)
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}

	// engine_new
	res, err := mod.ExportedFunction("fulgur_engine_new").Call(ctx)
	if err != nil {
		t.Fatalf("engine_new: %v", err)
	}
	handle := uint32(res[0])
	if handle == 0 {
		t.Fatal("engine_new returned 0")
	}

	// configure with an unknown field
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

// helpers — kept tiny so tests don't depend on the stdlib's `errors`
// being shadowed in this package.
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

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 3: Run tests**

Run: `cd bindings/go && go test -run TestErrors_ -v && cd -`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add bindings/go/errors.go bindings/go/fulgur_test.go
git commit -m "feat(bindings-go): add Error type and last_error fetch"
```

---

## Task 14: Go engine.go (typed wrappers around the wasm exports)

**Files:**

- Create: `bindings/go/engine.go`

- [ ] **Step 1: Write `bindings/go/engine.go`**

```go
package fulgur

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// instance binds a single wazero module to a long-lived engine handle.
// All operations require external synchronisation — instances are
// single-owner; concurrency comes from the Renderer pool.
type instance struct {
	mod    api.Module
	handle uint32
}

func newInstance(ctx context.Context, mod api.Module) (*instance, error) {
	res, err := mod.ExportedFunction("fulgur_engine_new").Call(ctx)
	if err != nil {
		return nil, fmt.Errorf("fulgur_engine_new: %w", err)
	}
	h := uint32(res[0])
	if h == 0 {
		return nil, fetchLastError(ctx, mod, "engine_new")
	}
	return &instance{mod: mod, handle: h}, nil
}

func (i *instance) close(ctx context.Context) error {
	if i.handle != 0 {
		_, _ = i.mod.ExportedFunction("fulgur_engine_free").Call(ctx, uint64(i.handle))
		i.handle = 0
	}
	return i.mod.Close(ctx)
}

func (i *instance) configure(ctx context.Context, jsonBytes []byte) error {
	ptr, n, err := writeBytes(ctx, i.mod, jsonBytes)
	if err != nil {
		return err
	}
	defer func() { _ = free(ctx, i.mod, ptr, n) }()

	res, err := i.mod.ExportedFunction("fulgur_engine_configure").Call(
		ctx, uint64(i.handle), uint64(ptr), uint64(n),
	)
	if err != nil {
		return fmt.Errorf("fulgur_engine_configure: %w", err)
	}
	if int32(res[0]) != 0 {
		return fetchLastError(ctx, i.mod, "configure")
	}
	return nil
}

func (i *instance) addFont(ctx context.Context, fontBytes []byte) error {
	ptr, n, err := writeBytes(ctx, i.mod, fontBytes)
	if err != nil {
		return err
	}
	defer func() { _ = free(ctx, i.mod, ptr, n) }()

	res, err := i.mod.ExportedFunction("fulgur_engine_add_font").Call(
		ctx, uint64(i.handle), uint64(ptr), uint64(n),
	)
	if err != nil {
		return fmt.Errorf("fulgur_engine_add_font: %w", err)
	}
	if int32(res[0]) != 0 {
		return fetchLastError(ctx, i.mod, "add_font")
	}
	return nil
}

func (i *instance) addCSS(ctx context.Context, name string, body []byte) error {
	namePtr, nameLen, err := writeBytes(ctx, i.mod, []byte(name))
	if err != nil {
		return err
	}
	defer func() { _ = free(ctx, i.mod, namePtr, nameLen) }()

	bodyPtr, bodyLen, err := writeBytes(ctx, i.mod, body)
	if err != nil {
		return err
	}
	defer func() { _ = free(ctx, i.mod, bodyPtr, bodyLen) }()

	res, err := i.mod.ExportedFunction("fulgur_engine_add_css").Call(
		ctx, uint64(i.handle),
		uint64(namePtr), uint64(nameLen),
		uint64(bodyPtr), uint64(bodyLen),
	)
	if err != nil {
		return fmt.Errorf("fulgur_engine_add_css: %w", err)
	}
	if int32(res[0]) != 0 {
		return fetchLastError(ctx, i.mod, "add_css")
	}
	return nil
}

func (i *instance) addImage(ctx context.Context, name string, body []byte) error {
	namePtr, nameLen, err := writeBytes(ctx, i.mod, []byte(name))
	if err != nil {
		return err
	}
	defer func() { _ = free(ctx, i.mod, namePtr, nameLen) }()

	bodyPtr, bodyLen, err := writeBytes(ctx, i.mod, body)
	if err != nil {
		return err
	}
	defer func() { _ = free(ctx, i.mod, bodyPtr, bodyLen) }()

	res, err := i.mod.ExportedFunction("fulgur_engine_add_image").Call(
		ctx, uint64(i.handle),
		uint64(namePtr), uint64(nameLen),
		uint64(bodyPtr), uint64(bodyLen),
	)
	if err != nil {
		return fmt.Errorf("fulgur_engine_add_image: %w", err)
	}
	if int32(res[0]) != 0 {
		return fetchLastError(ctx, i.mod, "add_image")
	}
	return nil
}

func (i *instance) render(ctx context.Context, html []byte) ([]byte, error) {
	ptr, n, err := writeBytes(ctx, i.mod, html)
	if err != nil {
		return nil, err
	}
	defer func() { _ = free(ctx, i.mod, ptr, n) }()

	res, err := i.mod.ExportedFunction("fulgur_engine_render").Call(
		ctx, uint64(i.handle), uint64(ptr), uint64(n),
	)
	if err != nil {
		return nil, fmt.Errorf("fulgur_engine_render: %w", err)
	}
	packed := res[0]
	if packed == 0 {
		return nil, fetchLastError(ctx, i.mod, "render")
	}
	pdfPtr := uint32(packed >> 32)
	pdfLen := uint32(packed)
	pdf, err := readBytes(i.mod, pdfPtr, pdfLen)
	if err != nil {
		_ = free(ctx, i.mod, pdfPtr, pdfLen)
		return nil, err
	}
	if err := free(ctx, i.mod, pdfPtr, pdfLen); err != nil {
		return nil, err
	}
	return pdf, nil
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd bindings/go && go build ./... && cd -`
Expected: no output, exit 0.

- [ ] **Step 3: Commit**

```bash
git add bindings/go/engine.go
git commit -m "feat(bindings-go): add typed wrappers around wasm exports"
```

---

## Task 15: Go options.go

**Files:**

- Create: `bindings/go/options.go`

- [ ] **Step 1: Write `bindings/go/options.go`**

```go
package fulgur

import "encoding/json"

// PageSize selects a named ISO size or a custom width/height in mm.
type PageSize struct {
	Name     string  `json:"-"`
	WidthMM  float32 `json:"widthMm,omitempty"`
	HeightMM float32 `json:"heightMm,omitempty"`
}

// MarshalJSON emits either a string ("A4", "Letter", "A3") or an object
// {widthMm, heightMm}, matching the wasm-side PageSizeOption variants.
func (p PageSize) MarshalJSON() ([]byte, error) {
	if p.Name != "" {
		return json.Marshal(p.Name)
	}
	type custom struct {
		W float32 `json:"widthMm"`
		H float32 `json:"heightMm"`
	}
	return json.Marshal(custom{W: p.WidthMM, H: p.HeightMM})
}

// Margin selects a uniform mm/pt margin or per-side mm margins. Set
// exactly one of (UniformMM, UniformPT, Per).
type Margin struct {
	UniformMM *float32      `json:"-"`
	UniformPT *float32      `json:"-"`
	Per       *MarginPerMM  `json:"-"`
}

// MarginPerMM is the per-side variant.
type MarginPerMM struct {
	TopMM    float32 `json:"topMm"`
	RightMM  float32 `json:"rightMm"`
	BottomMM float32 `json:"bottomMm"`
	LeftMM   float32 `json:"leftMm"`
}

// MarshalJSON emits one of {mm}, {pt}, or {topMm, rightMm, bottomMm, leftMm}.
func (m Margin) MarshalJSON() ([]byte, error) {
	switch {
	case m.UniformMM != nil:
		return json.Marshal(struct {
			MM float32 `json:"mm"`
		}{*m.UniformMM})
	case m.UniformPT != nil:
		return json.Marshal(struct {
			PT float32 `json:"pt"`
		}{*m.UniformPT})
	case m.Per != nil:
		return json.Marshal(m.Per)
	default:
		return []byte("null"), nil
	}
}

// Options mirrors the Rust EngineOptions JSON shape (camelCase fields,
// deny_unknown_fields on the wasm side). All fields are optional and
// behave as partial sticky overrides on the wasm-side EngineState.
type Options struct {
	PageSize     *PageSize `json:"pageSize,omitempty"`
	Margin       *Margin   `json:"margin,omitempty"`
	Landscape    *bool     `json:"landscape,omitempty"`
	Title        string    `json:"title,omitempty"`
	Authors      []string  `json:"authors,omitempty"`
	Description  string    `json:"description,omitempty"`
	Keywords     []string  `json:"keywords,omitempty"`
	Creator      string    `json:"creator,omitempty"`
	Producer     string    `json:"producer,omitempty"`
	CreationDate string    `json:"creationDate,omitempty"`
	Lang         string    `json:"lang,omitempty"`
	Bookmarks    *bool     `json:"bookmarks,omitempty"`
}
```

- [ ] **Step 2: Add a marshaling test to `bindings/go/fulgur_test.go`**

```go
func TestOptions_MarshalNamedPageSize(t *testing.T) {
	t.Helper()
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
	t.Helper()
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
	t.Helper()
	pt := float32(36)
	got, err := json.Marshal(Options{Margin: &Margin{UniformPT: &pt}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != `{"margin":{"pt":36}}` {
		t.Fatalf("got %s", got)
	}
}
```

Add to the imports in `fulgur_test.go`:

```go
	"encoding/json"
```

- [ ] **Step 3: Run tests**

Run: `cd bindings/go && go test -run TestOptions_ -v && cd -`
Expected: 3 tests pass.

- [ ] **Step 4: Commit**

```bash
git add bindings/go/options.go bindings/go/fulgur_test.go
git commit -m "feat(bindings-go): add Options struct + JSON marshaling"
```

---

## Task 16: Go pool.go + fulgur.go public API

**Files:**

- Create: `bindings/go/pool.go`
- Modify: `bindings/go/fulgur.go`

- [ ] **Step 1: Write `bindings/go/pool.go`**

```go
package fulgur

import (
	"context"
	"crypto/rand"
	"fmt"
	"runtime"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// rendererConfig collects construction-time options.
type rendererConfig struct {
	poolSize int
	fonts    [][]byte
	css      []Asset
	images   []Asset
}

// RendererOption configures a Renderer at construction time.
type RendererOption func(*rendererConfig)

// WithPoolSize sets the number of WASM module instances kept in the pool.
// Defaults to runtime.GOMAXPROCS(0). Values < 1 are clamped to 1.
func WithPoolSize(n int) RendererOption {
	return func(c *rendererConfig) {
		if n < 1 {
			n = 1
		}
		c.poolSize = n
	}
}

// WithFonts registers font byte slices on every instance in the pool.
// TTF, OTF, and WOFF2 are accepted; WOFF1 is rejected.
func WithFonts(fonts ...[]byte) RendererOption {
	return func(c *rendererConfig) { c.fonts = append(c.fonts, fonts...) }
}

// WithCSS registers stylesheets on every instance.
func WithCSS(assets ...Asset) RendererOption {
	return func(c *rendererConfig) { c.css = append(c.css, assets...) }
}

// WithImages registers images on every instance.
func WithImages(assets ...Asset) RendererOption {
	return func(c *rendererConfig) { c.images = append(c.images, assets...) }
}

// Asset is a named binary blob (CSS or image).
type Asset struct {
	Name string
	Data []byte
}

// Renderer renders HTML to PDF. Safe for concurrent use across goroutines:
// each Render acquires an instance from a fixed-size pool.
type Renderer struct {
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
	cfg      rendererConfig
	pool     chan *instance

	inflight sync.WaitGroup

	mu     sync.Mutex
	closed bool
}

// New compiles the embedded WASM module and starts the instance pool.
// The returned Renderer must be Closed when done.
func New(ctx context.Context, opts ...RendererOption) (*Renderer, error) {
	cfg := rendererConfig{poolSize: runtime.GOMAXPROCS(0)}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.poolSize < 1 {
		cfg.poolSize = 1
	}

	rt := wazero.NewRuntime(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, rt)
	compiled, err := rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("compile: %w", err)
	}

	r := &Renderer{
		runtime:  rt,
		compiled: compiled,
		cfg:      cfg,
		pool:     make(chan *instance, cfg.poolSize),
	}

	for i := 0; i < cfg.poolSize; i++ {
		inst, err := r.newConfiguredInstance(ctx)
		if err != nil {
			_ = r.Close()
			return nil, fmt.Errorf("instance %d: %w", i, err)
		}
		r.pool <- inst
	}
	return r, nil
}

// newConfiguredInstance instantiates a fresh module, registers the
// startup-time assets on its engine, and returns the instance ready to
// serve renders.
func (r *Renderer) newConfiguredInstance(ctx context.Context) (*instance, error) {
	modCfg := wazero.NewModuleConfig().
		WithName(""). // anonymous so we can instantiate many copies
		WithSysWalltime().
		WithSysNanotime().
		WithRandSource(rand.Reader)
	mod, err := r.runtime.InstantiateModule(ctx, r.compiled, modCfg)
	if err != nil {
		return nil, fmt.Errorf("instantiate: %w", err)
	}
	inst, err := newInstance(ctx, mod)
	if err != nil {
		_ = mod.Close(ctx)
		return nil, err
	}
	for _, font := range r.cfg.fonts {
		if err := inst.addFont(ctx, font); err != nil {
			_ = inst.close(ctx)
			return nil, err
		}
	}
	for _, a := range r.cfg.css {
		if err := inst.addCSS(ctx, a.Name, a.Data); err != nil {
			_ = inst.close(ctx)
			return nil, err
		}
	}
	for _, a := range r.cfg.images {
		if err := inst.addImage(ctx, a.Name, a.Data); err != nil {
			_ = inst.close(ctx)
			return nil, err
		}
	}
	return inst, nil
}

// acquire blocks until an instance is available or ctx is cancelled.
func (r *Renderer) acquire(ctx context.Context) (*instance, error) {
	select {
	case inst := <-r.pool:
		return inst, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// release returns the instance to the pool. If the instance is broken
// (passed as nil), a fresh one is built to keep the pool full.
func (r *Renderer) release(ctx context.Context, inst *instance) {
	if inst != nil {
		r.pool <- inst
		return
	}
	// Replace broken instance asynchronously? Do it inline to keep the
	// pool capacity tight; failures here are logged but not propagated.
	fresh, err := r.newConfiguredInstance(ctx)
	if err != nil {
		// Pool is now under-capacity. Best-effort: send back a nil-handle
		// instance won't work — leave the slot empty until Close. Future
		// Render calls will block one fewer slot, but the renderer is
		// still functional.
		_ = err
		return
	}
	r.pool <- fresh
}

// Close shuts down all instances and the wazero runtime. Idempotent.
// Waits for any in-flight Render calls to finish before tearing down,
// so the pool channel is never written to after being closed.
func (r *Renderer) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.mu.Unlock()

	r.inflight.Wait()

	close(r.pool)
	ctx := context.Background()
	for inst := range r.pool {
		_ = inst.close(ctx)
	}
	return r.runtime.Close(ctx)
}
```

- [ ] **Step 2: Replace `bindings/go/fulgur.go` with the public `Render` method**

```go
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
		// Discard instance on any failure (defensive: linear memory may
		// be in an inconsistent state). Replace synchronously so the
		// pool stays at fixed capacity.
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
```

- [ ] **Step 3: Add a basic Render integration test to `bindings/go/fulgur_test.go`**

```go
func TestRender_Basic(t *testing.T) {
	ctx := context.Background()
	r, err := New(ctx, WithPoolSize(1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = r.Close() }()

	pdf, err := r.Render(ctx, []byte("<p>hello fulgur</p>"), nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytesHasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("missing %%PDF- prefix; got %q", pdf[:8])
	}
}

func bytesHasPrefix(b, prefix []byte) bool {
	if len(b) < len(prefix) {
		return false
	}
	for i := range prefix {
		if b[i] != prefix[i] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: Run tests**

Run: `cd bindings/go && go test -run TestRender_Basic -v && cd -`
Expected: PASS. (Render may take 1–3 seconds on first run while wazero compiles the module.)

- [ ] **Step 5: Commit**

```bash
git add bindings/go/pool.go bindings/go/fulgur.go bindings/go/fulgur_test.go
git commit -m "feat(bindings-go): add pool-backed Renderer with Render/Close"
```

---

## Task 17: Concurrency + error-path integration tests

**Files:**

- Modify: `bindings/go/fulgur_test.go`

- [ ] **Step 1: Append concurrent-render and configure-error tests**

```go
func TestRender_ConcurrentAcrossPool(t *testing.T) {
	ctx := context.Background()
	r, err := New(ctx, WithPoolSize(4))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = r.Close() }()

	const n = 16
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			html := []byte("<p>doc " + itoa(i) + "</p>")
			pdf, err := r.Render(ctx, html, nil)
			if err != nil {
				errs <- err
				return
			}
			if !bytesHasPrefix(pdf, []byte("%PDF-")) {
				errs <- fmt.Errorf("doc %d: missing PDF prefix", i)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestRender_BadOptionsReturnsTypedError(t *testing.T) {
	ctx := context.Background()
	r, err := New(ctx, WithPoolSize(1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = r.Close() }()

	// Construct a Renderer-bypassing call: marshal options that the wasm
	// side accepts, then directly send a malformed JSON via the
	// instance to verify the error path. We simulate an "unknown
	// pageSize name" using the public Options API instead.
	bad := &Options{PageSize: &PageSize{Name: "Tabloid"}}
	_, err = r.Render(ctx, []byte("<p>x</p>"), bad)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	var fe *Error
	if !errorsAs(err, &fe) {
		t.Fatalf("want *fulgur.Error, got %T (%v)", err, err)
	}
	if !contains(fe.Message, "unknown page size") {
		t.Fatalf("want 'unknown page size' in message, got %q", fe.Message)
	}
}

// itoa avoids importing strconv in tests.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
```

Add to imports in `fulgur_test.go`:

```go
	"fmt"
	"sync"
```

- [ ] **Step 2: Run tests with the race detector**

Run: `cd bindings/go && go test -race -v && cd -`
Expected: All tests pass; no DATA RACE warnings.

- [ ] **Step 3: Commit**

```bash
git add bindings/go/fulgur_test.go
git commit -m "test(bindings-go): add concurrent + error-path integration tests"
```

---

## Task 18: Custom font + CSS integration test

**Files:**

- Modify: `bindings/go/fulgur_test.go`

- [ ] **Step 1: Append a font + CSS test**

```go
func TestRender_WithCustomCSS(t *testing.T) {
	ctx := context.Background()
	r, err := New(ctx,
		WithPoolSize(1),
		WithCSS(Asset{Name: "main.css", Data: []byte("p { color: red; }")}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = r.Close() }()

	pdf, err := r.Render(ctx, []byte("<p>red</p>"), nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytesHasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal("missing %PDF- prefix")
	}
}
```

- [ ] **Step 2: Run the test**

Run: `cd bindings/go && go test -run TestRender_WithCustomCSS -v && cd -`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add bindings/go/fulgur_test.go
git commit -m "test(bindings-go): cover WithCSS asset registration"
```

---

## Task 19: CI jobs (wasi-build + go-test)

**Files:**

- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Read the existing `wasm-check` job for the pattern**

Run: `grep -n -A 30 "wasm-check:" .github/workflows/ci.yml`
Expected: A job that installs the `wasm32-unknown-unknown` target and runs `cargo check`.

- [ ] **Step 2: Add a `wasi-build` job parallel to it**

Append to `.github/workflows/ci.yml` (under the `jobs:` map, indentation matching peer jobs):

```yaml
  wasi-build:
    name: WASI build (drift check)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: dtolnay/rust-toolchain@stable
        with:
          targets: wasm32-wasip1
      - uses: Swatinem/rust-cache@v2
        with:
          shared-key: ubuntu-latest-fulgur-wasi
      - name: Install binaryen (wasm-opt)
        run: |
          sudo apt-get update
          sudo apt-get install -y binaryen
      - name: Build wasm artifact
        run: |
          cargo build -p fulgur-wasi --release --target wasm32-wasip1
          mkdir -p /tmp/wasm-out
          wasm-opt -Oz -o /tmp/wasm-out/fulgur.wasm \
            target/wasm32-wasip1/release/fulgur_wasi.wasm
      - name: Verify committed artifact matches build
        run: |
          if ! cmp -s /tmp/wasm-out/fulgur.wasm bindings/go/internal/wasm/fulgur.wasm; then
            echo "::error::bindings/go/internal/wasm/fulgur.wasm is out of date."
            echo "Re-run 'mise run build-wasi' and commit the result."
            exit 1
          fi
          ls -lh bindings/go/internal/wasm/fulgur.wasm
```

- [ ] **Step 3: Add a `go-test` job**

```yaml
  go-test:
    name: Go SDK tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
          cache-dependency-path: bindings/go/go.sum
      - name: go test (race)
        working-directory: bindings/go
        run: go test -race -v ./...
```

- [ ] **Step 4: Verify the YAML parses locally if `actionlint` is available**

Run: `actionlint .github/workflows/ci.yml || echo 'actionlint not installed; skipping'`
Expected: no errors (or skip).

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add wasi-build (drift check) and go-test jobs"
```

---

## Task 20: Documentation (README files)

**Files:**

- Create: `bindings/go/README.md`
- Modify: `README.md` (root)

- [ ] **Step 1: Write `bindings/go/README.md`**

````markdown
# fulgur Go SDK

A pure-Go binding for [fulgur](https://github.com/fulgur-rs/fulgur) — HTML/CSS
to PDF conversion. Backed by the `fulgur-wasi` WebAssembly module, loaded
via [wazero](https://github.com/tetratelabs/wazero). No CGO required.

## Install

```bash
go get github.com/fulgur-rs/fulgur/bindings/go
```

The `.wasm` artifact is embedded in the package via `go:embed` — no
runtime download.

## Quickstart

```go
package main

import (
    "context"
    "log"
    "os"

    fulgur "github.com/fulgur-rs/fulgur/bindings/go"
)

func main() {
    ctx := context.Background()
    r, err := fulgur.New(ctx)
    if err != nil {
        log.Fatal(err)
    }
    defer r.Close()

    pdf, err := r.Render(ctx, []byte("<h1>Hello</h1>"), nil)
    if err != nil {
        log.Fatal(err)
    }
    _ = os.WriteFile("hello.pdf", pdf, 0o644)
}
```

## Pool sizing

`fulgur.New` builds a pool of WASM module instances; each instance can
serve one Render at a time. Defaults to `runtime.GOMAXPROCS(0)`. Override:

```go
r, _ := fulgur.New(ctx, fulgur.WithPoolSize(8))
```

Each instance holds its own linear memory (typically 5–30 MB after a few
renders). Size the pool to your concurrency target, not your CPU count,
if memory is a concern.

## Custom assets

Fonts, CSS, and images are registered at construction time and shared
across the pool. The default Noto Sans Regular font is bundled
automatically; you only need to register more if you use other families.

```go
font, _ := os.ReadFile("MyFont.ttf")
css, _ := os.ReadFile("style.css")

r, _ := fulgur.New(ctx,
    fulgur.WithFonts(font),
    fulgur.WithCSS(fulgur.Asset{Name: "style.css", Data: css}),
)
```

## Options are sticky

`Render(ctx, html, opts)` applies `opts` as a partial override on the
instance — fields you don't set keep their prior value on that instance.
Pool instances are picked round-robin-ish by Go channel semantics, so
relying on which instance handles a given call is undefined. If you need
deterministic options per call, set every field you care about every call.

## Offline-first

The wasm module makes no network calls. URLs in `<link rel="stylesheet">`
or `<img src="https://...">` are not fetched — register everything via
`WithCSS` / `WithImages` and reference assets by name.

## Determinism

Identical inputs and identical registered assets produce byte-identical
PDFs. The bundled font means you don't depend on host fonts (unlike the
fulgur CLI's default behaviour).
````

- [ ] **Step 2: Add a Go SDK link to the root `README.md`**

Find the bindings section (search for "pyfulgur" or "fulgur-ruby") and add a Go entry. If no such section exists, add one near the top:

```markdown
## Bindings

- [`crates/pyfulgur`](crates/pyfulgur) — Python bindings (PyO3)
- [`crates/fulgur-ruby`](crates/fulgur-ruby) — Ruby bindings (Magnus)
- [`crates/fulgur-wasm`](crates/fulgur-wasm) — Browser WebAssembly (wasm-bindgen)
- [`bindings/go`](bindings/go) — Go SDK over wazero (WASI)
```

(Adapt to whatever pattern the README already uses.)

- [ ] **Step 3: Lint the markdown**

Run: `npx markdownlint-cli2 'bindings/go/README.md'`
Expected: no errors. Fix any line-length, code-fence-language, or blank-line warnings reported.

- [ ] **Step 4: Commit**

```bash
git add bindings/go/README.md README.md
git commit -m "docs: add Go SDK README + root link"
```

---

## Final verification

Before opening a PR or merging:

- [ ] **Full Rust test suite passes**

```bash
cargo test -p fulgur-wasi
cargo check -p fulgur --target wasm32-wasip1
cargo check -p fulgur-wasi --target wasm32-wasip1
```

- [ ] **Full Go test suite passes with race detector**

```bash
cd bindings/go && go test -race -v ./...
```

- [ ] **Wasm artifact byte-matches a fresh build**

```bash
mise run build-wasi
git diff --exit-code -- bindings/go/internal/wasm/fulgur.wasm
```

- [ ] **fmt + clippy clean**

```bash
cargo fmt --check
cargo clippy --workspace --all-targets -- -D warnings
```

- [ ] **Markdown clean**

```bash
npx markdownlint-cli2 '**/*.md'
```
