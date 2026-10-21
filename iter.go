package gio

import (
	"iter"
	"sync"
)

// Items returns an [iter.Seq2] iterator that yields individual T items read from r.
// Iteration terminates cleanly when r returns [EOF]. If a non-EOF error is encountered,
// it yields the zero value of T along with that error and stops.
func Items[T any](r Reader[T]) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		if ir, ok := r.(ItemReader[T]); ok {
			for {
				item, err := ir.ReadItem()
				if err != nil {
					if err == EOF {
						return
					}
					var zero T
					yield(zero, err)
					return
				}
				if !yield(item, nil) {
					return
				}
			}
		}

		buf := make([]T, defaultBufferSize[T](4096))
		for {
			n, err := r.Read(buf)
			for i := range n {
				if !yield(buf[i], nil) {
					return
				}
			}
			if err != nil {
				if err == EOF {
					return
				}
				var zero T
				yield(zero, err)
				return
			}
		}
	}
}

// Chunks returns an [iter.Seq2] iterator that yields slices of up to size T items read from r.
// If size <= 0, a default buffer size is used.
// Note that the yielded slice is backed by an internal buffer reused across iterations.
func Chunks[T any](r Reader[T], size int) iter.Seq2[[]T, error] {
	if size <= 0 {
		size = defaultBufferSize[T](4096)
	}
	return func(yield func([]T, error) bool) {
		buf := make([]T, size)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				if !yield(buf[:n], nil) {
					return
				}
			}
			if err != nil {
				if err == EOF {
					return
				}
				yield(nil, err)
				return
			}
		}
	}
}

// FromSeq converts a push-based [iter.Seq] into a pull-based [ReadCloser].
// Calling Close releases the underlying pull iterator resources.
func FromSeq[T any](seq iter.Seq[T]) ReadCloser[T] {
	next, stop := iter.Pull(seq)
	return &seqReader[T]{next: next, stop: stop}
}

type seqReader[T any] struct {
	mu   sync.Mutex
	next func() (T, bool)
	stop func()
	done bool
}

func (r *seqReader[T]) Read(p []T) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return 0, EOF
	}
	for n < len(p) {
		v, ok := r.next()
		if !ok {
			r.done = true
			r.stop()
			if n == 0 {
				return 0, EOF
			}
			return n, nil
		}
		p[n] = v
		n++
	}
	return n, nil
}

func (r *seqReader[T]) ReadItem() (T, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		var zero T
		return zero, EOF
	}
	v, ok := r.next()
	if !ok {
		r.done = true
		r.stop()
		var zero T
		return zero, EOF
	}
	return v, nil
}

func (r *seqReader[T]) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.done {
		r.done = true
		r.stop()
	}
	return nil
}

// FromSeq2 converts an error-producing [iter.Seq2] into a pull-based [ReadCloser].
// Calling Close releases the underlying pull iterator resources.
func FromSeq2[T any](seq iter.Seq2[T, error]) ReadCloser[T] {
	next, stop := iter.Pull2(seq)
	return &seq2Reader[T]{next: next, stop: stop}
}

type seq2Reader[T any] struct {
	mu   sync.Mutex
	next func() (T, error, bool)
	stop func()
	done bool
}

func (r *seq2Reader[T]) Read(p []T) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return 0, EOF
	}
	for n < len(p) {
		v, iterErr, ok := r.next()
		if !ok {
			r.done = true
			r.stop()
			if n == 0 {
				return 0, EOF
			}
			return n, nil
		}
		if iterErr != nil {
			if iterErr == EOF {
				r.done = true
				r.stop()
				if n == 0 {
					return 0, EOF
				}
				return n, nil
			}
			return n, iterErr
		}
		p[n] = v
		n++
	}
	return n, nil
}

func (r *seq2Reader[T]) ReadItem() (T, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var zero T
	if r.done {
		return zero, EOF
	}
	v, iterErr, ok := r.next()
	if !ok {
		r.done = true
		r.stop()
		return zero, EOF
	}
	if iterErr != nil {
		if iterErr == EOF {
			r.done = true
			r.stop()
		}
		return zero, iterErr
	}
	return v, nil
}

func (r *seq2Reader[T]) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.done {
		r.done = true
		r.stop()
	}
	return nil
}
