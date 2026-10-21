package gio

const maxConsecutiveEmptyReads = 100

// MapReader returns a [Reader] that transforms each T item read from r into U using fn.
func MapReader[T, U any](r Reader[T], fn func(T) U) Reader[U] {
	return &mapReader[T, U]{
		r: r,
		fn: func(item T) (U, error) {
			return fn(item), nil
		},
	}
}

// TryMapReader returns a [Reader] that transforms each T item read from r into U using fn,
// stopping and returning any error produced by fn.
func TryMapReader[T, U any](r Reader[T], fn func(T) (U, error)) Reader[U] {
	return &mapReader[T, U]{r: r, fn: fn}
}

type mapReader[T, U any] struct {
	r   Reader[T]
	fn  func(T) (U, error)
	buf []T
}

func (m *mapReader[T, U]) Read(p []U) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	targetLen := min(len(p), defaultBufferSize[T](32*1024))
	if cap(m.buf) < targetLen {
		m.buf = make([]T, targetLen)
	} else {
		m.buf = m.buf[:targetLen]
	}

	nr, readErr := m.r.Read(m.buf)
	for i := range nr {
		u, mapErr := m.fn(m.buf[i])
		if mapErr != nil {
			clear(m.buf[:nr])
			return n, mapErr
		}
		p[n] = u
		n++
	}
	clear(m.buf[:nr])
	return n, readErr
}

func (m *mapReader[T, U]) ReadItem() (U, error) {
	item, err := ReadItem(m.r)
	if err != nil {
		var zero U
		return zero, err
	}
	return m.fn(item)
}

// MapWriter returns a [Writer] that transforms each written T item into U using fn
// and writes the resulting U items to w.
func MapWriter[T, U any](w Writer[U], fn func(T) U) Writer[T] {
	return &mapWriter[T, U]{
		w: w,
		fn: func(item T) (U, error) {
			return fn(item), nil
		},
	}
}

// TryMapWriter returns a [Writer] that transforms each written T item into U using fn
// and writes the resulting U items to w, stopping and returning any error produced by fn.
func TryMapWriter[T, U any](w Writer[U], fn func(T) (U, error)) Writer[T] {
	return &mapWriter[T, U]{w: w, fn: fn}
}

type mapWriter[T, U any] struct {
	w   Writer[U]
	fn  func(T) (U, error)
	buf []U
}

func (m *mapWriter[T, U]) Write(p []T) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	if cap(m.buf) < len(p) {
		m.buf = make([]U, len(p))
	} else {
		m.buf = m.buf[:len(p)]
	}

	valid := 0
	var mapErr error
	for _, item := range p {
		var u U
		u, mapErr = m.fn(item)
		if mapErr != nil {
			break
		}
		m.buf[valid] = u
		valid++
	}

	if valid > 0 {
		nw, writeErr := m.w.Write(m.buf[:valid])
		clear(m.buf[:valid])
		if writeErr != nil {
			return nw, writeErr
		}
		if nw != valid {
			return nw, ErrShortWrite
		}
		n = nw
	}
	return n, mapErr
}

func (m *mapWriter[T, U]) WriteItem(item T) error {
	u, err := m.fn(item)
	if err != nil {
		return err
	}
	return WriteItem(m.w, u)
}

// FilterReader returns a [Reader] that only yields T items from r for which pred(item) is true.
func FilterReader[T any](r Reader[T], pred func(T) bool) Reader[T] {
	return &filterReader[T]{r: r, pred: pred}
}

type filterReader[T any] struct {
	r    Reader[T]
	pred func(T) bool
}

func (f *filterReader[T]) Read(p []T) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	emptyReads := 0
	for {
		nr, readErr := f.r.Read(p)
		if nr == 0 && readErr == nil {
			emptyReads++
			if emptyReads >= maxConsecutiveEmptyReads {
				return 0, ErrNoProgress
			}
			continue
		}
		emptyReads = 0
		for i := range nr {
			if f.pred(p[i]) {
				p[n] = p[i]
				n++
			}
		}
		// Clear trailing scratch elements in p[n:nr] in case T contains pointers.
		if nr > n {
			clear(p[n:nr])
		}
		if n > 0 || readErr != nil {
			return n, readErr
		}
	}
}

func (f *filterReader[T]) ReadItem() (T, error) {
	for {
		item, err := ReadItem(f.r)
		if err != nil {
			var zero T
			return zero, err
		}
		if f.pred(item) {
			return item, nil
		}
	}
}

// FilterWriter returns a [Writer] that only forwards T items where pred(item) is true to w,
// while reporting the total number of input items processed to the caller.
func FilterWriter[T any](w Writer[T], pred func(T) bool) Writer[T] {
	return &filterWriter[T]{w: w, pred: pred}
}

type filterWriter[T any] struct {
	w    Writer[T]
	pred func(T) bool
	buf  []T
}

func (f *filterWriter[T]) Write(p []T) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	if cap(f.buf) < len(p) {
		f.buf = make([]T, 0, len(p))
	} else {
		f.buf = f.buf[:0]
	}
	for _, item := range p {
		if f.pred(item) {
			f.buf = append(f.buf, item)
		}
	}
	if len(f.buf) > 0 {
		nw, writeErr := f.w.Write(f.buf)
		clear(f.buf)
		if writeErr != nil {
			return nw, writeErr
		}
		if nw != len(f.buf) {
			return nw, ErrShortWrite
		}
	}
	return len(p), nil
}

func (f *filterWriter[T]) WriteItem(item T) error {
	if !f.pred(item) {
		return nil
	}
	return WriteItem(f.w, item)
}

// BatchReader returns a [Reader] of []T slices that groups T items read from r
// into batches of up to batchSize items. If batchSize <= 0, a default batch size is used.
func BatchReader[T any](r Reader[T], batchSize int) Reader[[]T] {
	if batchSize <= 0 {
		batchSize = defaultBufferSize[T](4096)
	}
	return &batchReader[T]{r: r, batchSize: batchSize}
}

type batchReader[T any] struct {
	r         Reader[T]
	batchSize int
}

func (b *batchReader[T]) Read(p [][]T) (n int, err error) {
	for n < len(p) {
		batch := make([]T, b.batchSize)
		m, readErr := ReadFull(b.r, batch)
		if m > 0 {
			p[n] = batch[:m]
			n++
		}
		if readErr != nil {
			if readErr == ErrUnexpectedEOF {
				if n < len(p) {
					return n, EOF
				}
				return n, nil
			}
			if readErr == EOF && n > 0 {
				return n, nil
			}
			return n, readErr
		}
	}
	return n, nil
}

// FlattenReader flattens a stream of []T batches into a contiguous [Reader] of T items.
func FlattenReader[T any](r Reader[[]T]) Reader[T] {
	return &flattenReader[T]{r: r}
}

type flattenReader[T any] struct {
	r   Reader[[]T]
	rem []T
}

func (f *flattenReader[T]) Read(p []T) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	emptyReads := 0
	for len(f.rem) == 0 {
		var batch [1][]T
		nr, readErr := f.r.Read(batch[:])
		if nr > 0 {
			f.rem = batch[0]
		}
		if len(f.rem) > 0 {
			break
		}
		if readErr != nil {
			return 0, readErr
		}
		emptyReads++
		if emptyReads >= maxConsecutiveEmptyReads {
			return 0, ErrNoProgress
		}
	}
	n = copy(p, f.rem)
	f.rem = f.rem[n:]
	return n, nil
}

// LimitWriter returns a [LimitedWriter] that writes to w but stops after n T items
// and returns [ErrShortWrite] (or LimitedWriter.Err if configured).
func LimitWriter[T any](w Writer[T], n int64) *LimitedWriter[T] {
	return &LimitedWriter[T]{W: w, N: n}
}

// A LimitedWriter writes to W but limits the amount of data written to just N T items.
// Each call to Write updates N to reflect the new amount remaining.
// Write returns Err (or [ErrShortWrite] if Err is nil) when N <= 0 and len(p) > 0.
type LimitedWriter[T any] struct {
	W   Writer[T] // underlying writer
	N   int64     // max T items remaining
	Err error     // error returned when limit is exceeded (defaults to ErrShortWrite)
}

func (l *LimitedWriter[T]) limitErr() error {
	if l.Err != nil {
		return l.Err
	}
	return ErrShortWrite
}

func (l *LimitedWriter[T]) Write(p []T) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	if l.N <= 0 {
		return 0, l.limitErr()
	}
	truncated := false
	if int64(len(p)) > l.N {
		p = p[:l.N]
		truncated = true
	}
	n, err = l.W.Write(p)
	l.N -= int64(n)
	if err == nil && truncated {
		err = l.limitErr()
	}
	return n, err
}

func (l *LimitedWriter[T]) WriteItem(item T) error {
	if l.N <= 0 {
		return l.limitErr()
	}
	if err := WriteItem(l.W, item); err != nil {
		return err
	}
	l.N--
	return nil
}
