package gio

import "slices"

var (
	_ ReadSeeker[any]      = (*SliceReader[any])(nil)
	_ ReaderAt[any]        = (*SliceReader[any])(nil)
	_ WriterTo[any]        = (*SliceReader[any])(nil)
	_ ReadItemScanner[any] = (*SliceReader[any])(nil)
	_ Writer[any]          = (*SliceWriter[any])(nil)
	_ WriterAt[any]        = (*SliceWriter[any])(nil)
	_ ReaderFrom[any]      = (*SliceWriter[any])(nil)
	_ ItemWriter[any]      = (*SliceWriter[any])(nil)
)

// A SliceReader implements the [Reader], [ReaderAt], [WriterTo], [Seeker],
// and [ItemScanner] interfaces by reading from a slice of T items.
// Unlike a buffer, a SliceReader is read-only and supports Seeking and ReadAt.
// The zero value for SliceReader operates like a SliceReader of an empty slice.
type SliceReader[T any] struct {
	s        []T
	i        int64 // current reading index
	prevItem int   // index of previous item; or < 0
}

// Len returns the number of T items of the unread portion of the slice.
func (r *SliceReader[T]) Len() int {
	if r.i >= int64(len(r.s)) {
		return 0
	}
	return int(int64(len(r.s)) - r.i)
}

// Size returns the original length of the underlying slice.
// Size is the number of T items available for reading via [SliceReader.ReadAt].
// The result is unaffected by any method calls except [SliceReader.Reset].
func (r *SliceReader[T]) Size() int64 { return int64(len(r.s)) }

// Read implements the [Reader] interface.
func (r *SliceReader[T]) Read(b []T) (n int, err error) {
	if r.i >= int64(len(r.s)) {
		return 0, EOF
	}
	r.prevItem = -1
	n = copy(b, r.s[r.i:])
	r.i += int64(n)
	return
}

// ReadAt implements the [ReaderAt] interface.
func (r *SliceReader[T]) ReadAt(b []T, off int64) (n int, err error) {
	// cannot modify state - see io.ReaderAt
	if off < 0 {
		return 0, ErrOffset
	}
	if off >= int64(len(r.s)) {
		return 0, EOF
	}
	n = copy(b, r.s[off:])
	if n < len(b) {
		err = EOF
	}
	return
}

// ReadItem implements the [ItemReader] interface.
func (r *SliceReader[T]) ReadItem() (T, error) {
	r.prevItem = -1
	if r.i >= int64(len(r.s)) {
		var zero T
		return zero, EOF
	}
	item := r.s[r.i]
	r.prevItem = int(r.i)
	r.i++
	return item, nil
}

// UnreadItem complements [SliceReader.ReadItem] in implementing the [ItemScanner] interface.
func (r *SliceReader[T]) UnreadItem() error {
	if r.i <= 0 || r.prevItem < 0 {
		return ErrInvalidUnread
	}
	r.prevItem = -1
	r.i--
	return nil
}

// Seek implements the [Seeker] interface.
func (r *SliceReader[T]) Seek(offset int64, whence int) (int64, error) {
	r.prevItem = -1
	var abs int64
	switch whence {
	case SeekStart:
		abs = offset
	case SeekCurrent:
		abs = r.i + offset
	case SeekEnd:
		abs = int64(len(r.s)) + offset
	default:
		return 0, ErrWhence
	}
	if abs < 0 {
		return 0, ErrOffset
	}
	r.i = abs
	return abs, nil
}

// WriteTo implements the [WriterTo] interface.
func (r *SliceReader[T]) WriteTo(w Writer[T]) (n int64, err error) {
	r.prevItem = -1
	if r.i >= int64(len(r.s)) {
		return 0, nil
	}
	b := r.s[r.i:]
	m, err := w.Write(b)
	if m > len(b) {
		panic("gio.SliceReader.WriteTo: invalid Write count")
	}
	r.i += int64(m)
	n = int64(m)
	if m != len(b) && err == nil {
		err = ErrShortWrite
	}
	return
}

// Reset resets the [SliceReader] to be reading from b.
func (r *SliceReader[T]) Reset(b []T) { *r = SliceReader[T]{b, 0, -1} }

// NewReader returns a new [SliceReader] reading from b.
func NewReader[T any](b []T) *SliceReader[T] { return &SliceReader[T]{b, 0, -1} }

// SliceWriter is an in-memory [Writer], [WriterAt], [ReaderFrom], and [ItemWriter]
// that appends or writes T items into a backing slice.
type SliceWriter[T any] struct {
	buf []T
}

// NewSliceWriter returns a new [SliceWriter] using buf as its initial slice.
// To preallocate capacity without initial elements, pass make([]T, 0, cap).
func NewSliceWriter[T any](buf []T) *SliceWriter[T] {
	return &SliceWriter[T]{buf: buf}
}

// Write appends p to the underlying slice.
func (w *SliceWriter[T]) Write(p []T) (int, error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}

// WriteItem appends a single item to the underlying slice.
func (w *SliceWriter[T]) WriteItem(item T) error {
	w.buf = append(w.buf, item)
	return nil
}

// WriteAt writes len(p) T items from p to the underlying slice at offset off,
// growing the slice if necessary.
func (w *SliceWriter[T]) WriteAt(p []T, off int64) (int, error) {
	if off < 0 {
		return 0, ErrOffset
	}
	end := int(off) + len(p)
	if end > len(w.buf) {
		if end > cap(w.buf) {
			w.buf = slices.Grow(w.buf[:cap(w.buf)], end-cap(w.buf))
		}
		w.buf = w.buf[:end]
	}
	copy(w.buf[off:], p)
	return len(p), nil
}

// ReadFrom reads all T items from r until EOF and appends them to the slice.
func (w *SliceWriter[T]) ReadFrom(r Reader[T]) (n int64, err error) {
	step := defaultBufferSize[T](512)
	for {
		if cap(w.buf)-len(w.buf) == 0 {
			w.buf = slices.Grow(w.buf, step)
		}
		m, readErr := r.Read(w.buf[len(w.buf):cap(w.buf)])
		if m < 0 {
			return n, ErrInvalidWrite
		}
		w.buf = w.buf[:len(w.buf)+m]
		n += int64(m)
		if readErr != nil {
			if readErr == EOF {
				return n, nil
			}
			return n, readErr
		}
	}
}

// Slice returns the accumulated slice of T items.
func (w *SliceWriter[T]) Slice() []T { return w.buf }

// Len returns the number of T items currently in the slice.
func (w *SliceWriter[T]) Len() int { return len(w.buf) }

// Cap returns the capacity of the underlying slice.
func (w *SliceWriter[T]) Cap() int { return cap(w.buf) }

// Reset clears the underlying slice while retaining its capacity for future writes.
func (w *SliceWriter[T]) Reset() {
	clear(w.buf)
	w.buf = w.buf[:0]
}
