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
	poolSize    int
	fonts       [][]byte
	css         []Asset
	images      []Asset
	interpreter bool
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

// WithInterpreter forces wazero's interpreter backend instead of the
// default (optimizing) JIT. Required on darwin/arm64 today: wazero's
// wazevo arm64 compiler panics on the fulgur.wasm module
// (resolveAddressingMode bug). Production callers on Linux x86_64 can
// leave this off for full JIT performance.
func WithInterpreter() RendererOption {
	return func(c *rendererConfig) { c.interpreter = true }
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

	var rt wazero.Runtime
	if cfg.interpreter {
		rt = wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter())
	} else {
		rt = wazero.NewRuntime(ctx)
	}
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
	fresh, err := r.newConfiguredInstance(ctx)
	if err != nil {
		// Pool slot is leaked; renderer keeps functioning at reduced
		// capacity. Subsequent Render calls block on one fewer slot.
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
