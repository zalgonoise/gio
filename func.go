package gio

import "context"

// ReaderFunc is an adapter to allow the use of ordinary functions as [Reader]s.
// If f is a function with the appropriate signature, ReaderFunc(f) is a [Reader]
// that calls f.
type ReaderFunc[T any] func(p []T) (n int, err error)

// Read calls f(p).
func (f ReaderFunc[T]) Read(p []T) (n int, err error) {
	return f(p)
}

// WriterFunc is an adapter to allow the use of ordinary functions as [Writer]s.
// If f is a function with the appropriate signature, WriterFunc(f) is a [Writer]
// that calls f.
type WriterFunc[T any] func(p []T) (n int, err error)

// Write calls f(p).
func (f WriterFunc[T]) Write(p []T) (n int, err error) {
	return f(p)
}

// ContextReader returns a [Reader] that checks ctx.Err() prior to each Read call.
// If the context is canceled or its deadline has elapsed, Read returns 0, ctx.Err()
// without invoking the underlying Reader.
func ContextReader[T any](ctx context.Context, r Reader[T]) Reader[T] {
	if ctx == nil {
		return r
	}
	return &contextReader[T]{ctx: ctx, r: r}
}

type contextReader[T any] struct {
	ctx context.Context
	r   Reader[T]
}

func (cr *contextReader[T]) Read(p []T) (int, error) {
	if err := cr.ctx.Err(); err != nil {
		return 0, err
	}
	return cr.r.Read(p)
}

func (cr *contextReader[T]) ReadItem() (T, error) {
	if err := cr.ctx.Err(); err != nil {
		var zero T
		return zero, err
	}
	return ReadItem(cr.r)
}

// ContextWriter returns a [Writer] that checks ctx.Err() prior to each Write call.
// If the context is canceled or its deadline has elapsed, Write returns 0, ctx.Err()
// without invoking the underlying Writer.
func ContextWriter[T any](ctx context.Context, w Writer[T]) Writer[T] {
	if ctx == nil {
		return w
	}
	return &contextWriter[T]{ctx: ctx, w: w}
}

type contextWriter[T any] struct {
	ctx context.Context
	w   Writer[T]
}

func (cw *contextWriter[T]) Write(p []T) (int, error) {
	if err := cw.ctx.Err(); err != nil {
		return 0, err
	}
	return cw.w.Write(p)
}

func (cw *contextWriter[T]) WriteItem(item T) error {
	if err := cw.ctx.Err(); err != nil {
		return err
	}
	return WriteItem(cw.w, item)
}
