package gio

import (
	"context"
	"sync"
)

// ChanReader adapts a receive-only channel <-chan T into a [Reader] and [ItemReader].
type ChanReader[T any] struct {
	ch  <-chan T
	ctx context.Context
}

// NewChanReader returns a new [ChanReader] that reads T items from ch.
// When ch is closed, Read and ReadItem return [EOF].
func NewChanReader[T any](ch <-chan T) *ChanReader[T] {
	return &ChanReader[T]{ch: ch}
}

// NewChanReaderContext returns a new [ChanReader] that reads T items from ch
// and aborts with ctx.Err() if ctx is canceled while waiting for items.
func NewChanReaderContext[T any](ctx context.Context, ch <-chan T) *ChanReader[T] {
	return &ChanReader[T]{ch: ch, ctx: ctx}
}

// Read reads up to len(p) T items from the channel into p.
// It blocks for the first item, then drains any immediately available items
// from the channel without blocking, adhering to the [Reader] contract.
func (r *ChanReader[T]) Read(p []T) (n int, err error) {
	if r.ch == nil {
		return 0, EOF
	}
	if r.ctx != nil {
		if err := r.ctx.Err(); err != nil {
			return 0, err
		}
	}
	if len(p) == 0 {
		return 0, nil
	}

	// Block for the first item.
	if r.ctx != nil {
		select {
		case <-r.ctx.Done():
			return 0, r.ctx.Err()
		case v, ok := <-r.ch:
			if !ok {
				return 0, EOF
			}
			p[0] = v
			n = 1
		}
	} else {
		v, ok := <-r.ch
		if !ok {
			return 0, EOF
		}
		p[0] = v
		n = 1
	}

	// Drain any immediately available items without blocking.
	for n < len(p) {
		select {
		case v, ok := <-r.ch:
			if !ok {
				return n, EOF
			}
			p[n] = v
			n++
		default:
			return n, nil
		}
	}
	return n, nil
}

// ReadItem reads and returns a single T item from the channel.
func (r *ChanReader[T]) ReadItem() (T, error) {
	var zero T
	if r.ch == nil {
		return zero, EOF
	}
	if r.ctx != nil {
		if err := r.ctx.Err(); err != nil {
			return zero, err
		}
		select {
		case <-r.ctx.Done():
			return zero, r.ctx.Err()
		case v, ok := <-r.ch:
			if !ok {
				return zero, EOF
			}
			return v, nil
		}
	}
	v, ok := <-r.ch
	if !ok {
		return zero, EOF
	}
	return v, nil
}

// ChanWriter adapts a send-only channel chan<- T into a [WriteCloser] and [ItemWriter].
type ChanWriter[T any] struct {
	ch     chan<- T
	ctx    context.Context
	mu     sync.RWMutex
	closed bool
}

// NewChanWriter returns a new [ChanWriter] that sends written T items to ch.
// Calling Close on the returned ChanWriter closes ch.
func NewChanWriter[T any](ch chan<- T) *ChanWriter[T] {
	return &ChanWriter[T]{ch: ch}
}

// NewChanWriterContext returns a new [ChanWriter] that sends written T items to ch
// and aborts with ctx.Err() if ctx is canceled while sending.
func NewChanWriterContext[T any](ctx context.Context, ch chan<- T) *ChanWriter[T] {
	return &ChanWriter[T]{ch: ch, ctx: ctx}
}

// Write sends each item in p to the underlying channel.
func (w *ChanWriter[T]) Write(p []T) (n int, err error) {
	if w.ch == nil {
		return 0, ErrClosedPipe
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.closed {
		return 0, ErrClosedPipe
	}
	if w.ctx != nil {
		if err := w.ctx.Err(); err != nil {
			return 0, err
		}
		for _, item := range p {
			select {
			case <-w.ctx.Done():
				return n, w.ctx.Err()
			case w.ch <- item:
				n++
			}
		}
		return n, nil
	}
	for _, item := range p {
		w.ch <- item
		n++
	}
	return n, nil
}

// WriteItem sends a single item to the underlying channel.
func (w *ChanWriter[T]) WriteItem(item T) error {
	if w.ch == nil {
		return ErrClosedPipe
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.closed {
		return ErrClosedPipe
	}
	if w.ctx != nil {
		if err := w.ctx.Err(); err != nil {
			return err
		}
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		case w.ch <- item:
			return nil
		}
	}
	w.ch <- item
	return nil
}

// Close closes the underlying channel. Subsequent calls to Close are no-ops,
// and subsequent calls to Write or WriteItem return [ErrClosedPipe].
func (w *ChanWriter[T]) Close() error {
	if w.ch == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		w.closed = true
		close(w.ch)
	}
	return nil
}
