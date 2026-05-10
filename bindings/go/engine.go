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
