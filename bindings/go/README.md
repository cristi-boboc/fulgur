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

That's the whole API. `New` automatically picks a wazero backend that
works on the current host (see [Backend selection](#backend-selection-jit-vs-interpreter)
below for details).

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

## Backend selection (JIT vs interpreter)

`fulgur.New(ctx)` picks the right wazero backend automatically:

- **arm64 (any OS)** — uses the interpreter directly. wazero's wazevo
  arm64 backend has a `resolveAddressingMode` bug that trips on the
  fulgur.wasm module, so the JIT probe is skipped at construction.
- **amd64 / other** — tries the optimizing JIT, and if it panics for any
  unforeseen reason, silently retries with the interpreter as a safety
  net.

You don't need to do platform detection in your code.

To override:

```go
r, _ := fulgur.New(ctx, fulgur.WithInterpreter()) // force interpreter
r, _ := fulgur.New(ctx, fulgur.WithJIT())         // force JIT, error on panic
```

`WithJIT()` is for callers who'd rather hard-fail than silently use the
slower interpreter.
