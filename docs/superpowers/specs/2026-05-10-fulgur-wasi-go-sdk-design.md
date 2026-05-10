# fulgur-wasi + Go SDK design

**Date:** 2026-05-10
**Status:** approved (pending spec review)
**Tracking:** related to fulgur-iym (strategic v0.7.0 WASM workstream)

## Summary

Add a wazero-compatible WebAssembly build of fulgur and a Go SDK that wraps
it, enabling serverside HTML→PDF generation from Go without CGO or a separate
fulgur binary. Two new units land in this repo:

1. **`crates/fulgur-wasi/`** — Rust cdylib targeting `wasm32-wasip1`. Exposes
   a small C ABI over linear memory. Sibling to existing `fulgur-wasm`,
   `pyfulgur`, `fulgur-ruby`.
2. **`bindings/go/`** — Go module embedding the prebuilt `.wasm` via
   `go:embed`. Provides a pool-backed `Renderer` whose `Render` method is
   goroutine-safe for serverside HTTP handlers.

The existing browser-targeted `crates/fulgur-wasm` (wasm-bindgen) is left
untouched. wazero cannot load wasm-bindgen output.

## Goals and non-goals

**Goals**

- Pure-Go integration: no CGO, no separate fulgur process, no JS runtime.
- Concurrent-safe `Render` for HTTP handler use.
- Offline-first: no network, no system-font reliance. A built-in default
  font ships embedded in the `.wasm`; callers can register more.
- Deterministic byte output for repeated renders with identical inputs and
  registered assets.

**Non-goals**

- Streaming PDF output. Render returns one `[]byte`.
- File-system access from inside the wasm module.
- Runtime hot-swap of the embedded font.
- Public stable C ABI for use by other languages. The ABI exists to serve
  the Go SDK; treating it as a public binding interface is out of scope
  for this spec.

## Architecture

```text
Go application
    │ pure-Go
    ▼
bindings/go/                          wazero runtime (WASI Preview 1)
  fulgur.go            ──────────▶      compiled module cache
  pool.go                                           │
  //go:embed *.wasm                                 ▼
                                       fulgur.wasm (wasm32-wasip1)
                                         exports: alloc, free,
                                           engine_new, engine_…,
                                           render, last_error
                                         embedded default font
                                                    │
                                                    ▼
                                       crates/fulgur-wasi
                                         thin C-ABI shim over
                                         fulgur::Engine
```

**Invariant:** one wazero module instance owns one Rust `Engine` lifecycle.
Pool size in the Go SDK bounds total memory.

## Rust crate `fulgur-wasi`

### Layout

```text
crates/fulgur-wasi/
├── Cargo.toml
└── src/
    ├── lib.rs           // C ABI exports (extern "C", #[no_mangle])
    ├── engine.rs        // EngineState + slab handle storage
    ├── options.rs       // EngineOptions (mirrors fulgur-wasm shape)
    ├── memory.rs        // alloc/free helpers
    ├── error.rs         // last-error thread-local
    └── default_font.rs  // include_bytes! Noto Sans subset
```

### Cargo.toml skeleton

```toml
[package]
name = "fulgur-wasi"
version = "0.15.0"  # tracks workspace
edition.workspace = true
publish = false

[lib]
crate-type = ["cdylib"]

[dependencies]
fulgur = { path = "../fulgur" }
serde = { version = "1", features = ["derive"] }
serde_json = "1"
slab = "0.4"
```

No `wasm-bindgen`, no `js-sys`. Browser-side bundles remain in
`crates/fulgur-wasm`.

### `getrandom` and time

Append a target block to `crates/fulgur/Cargo.toml`:

```toml
[target.wasm32-wasip1.dependencies]
getrandom = { version = "0.4", features = ["wasi"] }
```

This resolves the WASI backend selection noted in the existing comment in
`crates/fulgur/Cargo.toml`. Closes part of fulgur-iym.

`creation_date` defaults to `None` under WASI unless the caller sets it via
`configure`. Avoids relying on `wasi_snapshot_preview1.clock_time_get`
returning sensible host values.

### C ABI

All functions are `extern "C"` with `#[no_mangle]`. UTF-8 byte arrays are
passed by `(ptr, len)` pairs — no null termination. All errors flow through
the `last_error_*` slot.

```rust
// Memory management
fn fulgur_alloc(len: u32) -> u32;            // returns ptr; 0 = OOM
fn fulgur_free(ptr: u32, len: u32);

// Engine lifecycle
fn fulgur_engine_new() -> u32;               // returns handle; 0 = error
fn fulgur_engine_free(handle: u32);

// Configuration (JSON, deny_unknown_fields, mirrors fulgur-wasm EngineOptions)
fn fulgur_engine_configure(handle: u32, json_ptr: u32, json_len: u32) -> i32;  // 0 ok, -1 err

// Asset registration (bytes already in wasm memory)
fn fulgur_engine_add_font(handle: u32, ptr: u32, len: u32) -> i32;
fn fulgur_engine_add_css (handle: u32, name_ptr: u32, name_len: u32, ptr: u32, len: u32) -> i32;
fn fulgur_engine_add_image(handle: u32, name_ptr: u32, name_len: u32, ptr: u32, len: u32) -> i32;

// Rendering: returns packed (ptr<<32 | len); 0 = err. Caller frees with fulgur_free.
fn fulgur_engine_render(handle: u32, html_ptr: u32, html_len: u32) -> u64;

// Last error
fn fulgur_last_error_len() -> u32;
fn fulgur_last_error_copy(dst_ptr: u32, dst_len: u32) -> u32;  // returns bytes copied
```

### Implementation notes

- `fulgur_alloc` / `fulgur_free` wrap `Vec::with_capacity(len).leak()` and
  `Vec::from_raw_parts(...).drop()`. Standard wazero-friendly pattern.
- Engines live in `thread_local! { static ENGINES: RefCell<Slab<EngineState>> }`.
  WASI Preview 1 is single-threaded per module instance, so this is safe and
  lock-free. Concurrency comes from instance pooling on the Go side.
- `last_error` is `thread_local! { static LAST_ERR: RefCell<String> }`.
  Every fallible call clears it on success and sets it on failure.
- `EngineOptions` JSON shape is the **same shape** as the `EngineOptions`
  struct in `crates/fulgur-wasm/src/lib.rs`, with `deny_unknown_fields`.
  Inlined per-crate (no shared bindings-common crate yet — keeps blast
  radius small).
- `fulgur_engine_new` automatically registers the embedded default font via
  `include_bytes!("../assets/NotoSans-Regular.ttf")`. Caller can register
  additional fonts on top.

## Go SDK `bindings/go/`

### Module layout

```text
bindings/go/
├── go.mod              // module github.com/fulgur-rs/fulgur/bindings/go
├── fulgur.go           // public API
├── pool.go             // instance pool (buffered channel)
├── memory.go           // alloc/free + read/write helpers
├── engine.go           // typed wrappers around exports
├── options.go          // Options struct + JSON marshaling
├── errors.go
├── internal/
│   └── wasm/
│       └── fulgur.wasm // committed artifact (CI-built on release)
├── fulgur_test.go
└── README.md
```

### Public API

```go
package fulgur

type Renderer struct { /* unexported */ }

type Options struct {
    PageSize    *PageSize  `json:"pageSize,omitempty"`
    Margin      *Margin    `json:"margin,omitempty"`
    Landscape   *bool      `json:"landscape,omitempty"`
    Title       string     `json:"title,omitempty"`
    Authors     []string   `json:"authors,omitempty"`
    Description string     `json:"description,omitempty"`
    Keywords    []string   `json:"keywords,omitempty"`
    Creator     string     `json:"creator,omitempty"`
    Producer    string     `json:"producer,omitempty"`
    CreationDate string    `json:"creationDate,omitempty"`
    Lang        string     `json:"lang,omitempty"`
    Bookmarks   *bool      `json:"bookmarks,omitempty"`
}

type Asset struct {
    Name string  // empty for fonts
    Data []byte
}

func New(ctx context.Context, opts ...RendererOption) (*Renderer, error)

type RendererOption func(*rendererConfig)

func WithPoolSize(n int) RendererOption       // default: runtime.GOMAXPROCS(0)
func WithFonts(fonts ...[]byte) RendererOption
func WithCSS(assets ...Asset) RendererOption
func WithImages(assets ...Asset) RendererOption

// Render is goroutine-safe. Acquires an instance from the pool.
func (r *Renderer) Render(ctx context.Context, html []byte, opts *Options) ([]byte, error)

func (r *Renderer) Close() error
```

### Pool semantics

- Backed by a buffered `chan *instance` of size N.
- `Render` does `select { case inst := <-ch: ...; case <-ctx.Done(): ... }`,
  uses the instance, then returns it on `ch <- inst`.
- Each `*instance` wraps a wazero `api.Module`, holds a long-lived engine
  handle, and re-applies per-call `Options` via `engine_configure`.
- Fonts/CSS/images registered in `New` are added once at instance startup
  and shared across all calls handled by that instance.
- **Options are sticky on an instance and partial-overriding.** Each field
  of `EngineOptions` is `Option<T>` on the Rust side; only fields present
  in the configure JSON override the instance's prior value (mirrors the
  existing `fulgur-wasm::Engine::apply_options` semantics). Subsequent
  calls with `opts == nil` inherit prior values. Callers that want a
  full reset can construct a `Renderer` with a fresh pool, or pass a
  `Options` value with every field they care about set. This is
  documented in the Go README.
- If **any** call during a `Render` (configure, alloc, render, free) fails,
  the instance is **discarded and replaced** (defensive: assume corrupt
  linear memory or leaked state). Pool stays at fixed capacity.

### Per-call flow inside `Render`

1. Acquire instance from pool.
2. If `opts != nil`: marshal `Options` to JSON, alloc into wasm memory, call
   `fulgur_engine_configure`. Otherwise keep prior config.
3. Alloc html bytes, copy in, call `fulgur_engine_render`.
4. Unpack `(ptr, len)` from packed `u64`, copy PDF bytes out of wasm
   memory, call `fulgur_free` on the result.
5. Free the html buffer; return instance to pool.

### Wazero runtime config

- One `wazero.Runtime` per `Renderer`, compiled once with
  `runtime.CompileModule(ctx, wasmBytes)`.
- `wasi_snapshot_preview1.MustInstantiate(ctx, runtime)` for WASI imports.
- `ModuleConfig().WithSysWalltime().WithSysNanotime().WithRandSource(rand.Reader)`.
- No filesystem, no stdin/stdout/stderr by default. Caller can opt in for
  debugging via a future `WithDebugLogging` option (out of scope for v1).

### Embedding

```go
//go:embed internal/wasm/fulgur.wasm
var wasmBytes []byte
```

### Error mapping

- Negative return / zero handle → call `fulgur_last_error_len` +
  `fulgur_last_error_copy` to fetch the message.
- Wrap into a typed `*fulgur.Error` with the message field. Class field
  reserved for future expansion.

## Build and distribution

The `.wasm` artifact is built once per release and **committed** to
`bindings/go/internal/wasm/fulgur.wasm`. `go get` requires no network beyond
the Go module fetch.

### Local build

`mise.toml` task `build-wasi`:

```bash
cargo build -p fulgur-wasi --release --target wasm32-wasip1
wasm-opt -Oz -o bindings/go/internal/wasm/fulgur.wasm \
  target/wasm32-wasip1/release/fulgur_wasi.wasm
```

`wasm-opt -Oz` from binaryen typically halves binary size. Expected output:
6–10 MB. See risk #1 below if this lands above 15 MB.

### CI

New job `wasi-build` (parallel to existing `wasm-check`):

- Installs `wasm32-wasip1` target + `wasm-opt`.
- Runs the `build-wasi` task.
- On PRs: rebuild, byte-compare against the committed artifact, fail on
  drift. Same philosophy as VRT goldens.
- On `main` post-merge: regenerate, commit if changed (mirrors the
  update-examples workflow).

New job `go-test`:

- Sets up Go 1.23+.
- `cd bindings/go && go test -race ./...`.

## Testing strategy

Three layers, mirroring `pyfulgur` / `fulgur-ruby`:

1. **Rust unit tests** in `crates/fulgur-wasi/src/lib.rs` — JSON parsing,
   slab handle lifecycle, error propagation. Run on the *native* target
   (`cargo test -p fulgur-wasi --lib`). The C ABI fns delegate to inner
   `_inner` helpers that are testable without wasm32.
2. **Go integration tests** in `bindings/go/fulgur_test.go` — load the
   embedded `.wasm` via wazero, render known HTML, assert the PDF magic
   header `%PDF-` and a non-zero page count. Cases:
   - basic single render
   - render with bundled font + custom CSS
   - concurrent renders across the pool (`go test -race`)
   - error path: malformed configure JSON rejected
   - `Close()` after `Render()` releases all instances
3. **Determinism smoke test** — render the same HTML twice, byte-compare
   PDFs. Mirrors `examples_determinism.rs`.

**Per CLAUDE.md guidance:** non-trivial Rust logic in `crates/fulgur-wasi`
gets coverage from the native unit tests, since the Go integration tests
do not feed codecov. Don't rely on Go tests for Rust patch coverage.

## Documentation

- `bindings/go/README.md` — quickstart, pool sizing guidance, font
  registration example, offline-first caveat, `Close()` discipline.
- One new section in the root `README.md` linking to `bindings/go/`.

## Risks and mitigations

1. **Binary size.** Krilla + Blitz + image decoders is heavy. If we land
   above 15 MB even after `wasm-opt -Oz`, drop the embedded font into a
   separate Go package (`bindings/go/font`) so the core SDK stays small.
2. **`std::time` calls in transitive deps.** Some deps may panic under
   WASI if they invoke clock APIs unexpectedly. Mitigation: comprehensive
   smoke test in CI; if a panic surfaces, patch with a feature flag.
3. **`fontdb::Database::load_system_fonts()` in `blitz-dom`** for inline
   `<svg>`. Under WASI there is no system font directory; this should
   silently produce an empty list, but worth a smoke test with HTML
   containing `<svg><text>` early.
4. **Compile time.** Adding `wasm32-wasip1` to the workspace adds CI
   minutes. Mitigated by parallel job + sccache.
5. **wazero version churn.** Pin a specific wazero version in `go.mod`
   and bump deliberately.

## Out of scope (future)

- Streaming PDF output via callback.
- A wrapper Go SDK that shells out to fulgur-cli (an alternative for
  callers who want a separate process boundary).
- WASI Preview 2 / component model — revisit when wazero support is
  stable and the WIT story is mature.
- Stable public C ABI for other-language consumers.

## References

- Existing browser bindings: `crates/fulgur-wasm/src/lib.rs`
- Existing non-JS bindings (pattern reference): `crates/pyfulgur/`,
  `crates/fulgur-ruby/`
- `crates/fulgur/Cargo.toml` — getrandom backend comment, fulgur-iym
- CLAUDE.md — coverage scope, Engine builder, fd 1 policy, offline-first
- wazero: <https://github.com/tetratelabs/wazero>
