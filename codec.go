package gio

import "io"

// NewDecodedReader wraps a standard [io.Reader] using a decode function that
// reads and decodes one T item per call until [EOF].
// The returned [Reader] also implements [ItemReader].
func NewDecodedReader[T any](r io.Reader, decode func(io.Reader) (T, error)) Reader[T] {
	return &decodedReader[T]{r: r, decode: decode}
}

type decodedReader[T any] struct {
	r      io.Reader
	decode func(io.Reader) (T, error)
}

func (d *decodedReader[T]) Read(p []T) (n int, err error) {
	for n < len(p) {
		item, decErr := d.decode(d.r)
		if decErr != nil {
			if decErr == EOF && n > 0 {
				return n, nil
			}
			return n, decErr
		}
		p[n] = item
		n++
	}
	return n, nil
}

func (d *decodedReader[T]) ReadItem() (T, error) {
	return d.decode(d.r)
}

// NewEncodedWriter wraps a standard [io.Writer] using an encode function that
// serializes and writes each T item to w.
// The returned [Writer] also implements [ItemWriter].
func NewEncodedWriter[T any](w io.Writer, encode func(io.Writer, T) error) Writer[T] {
	return &encodedWriter[T]{w: w, encode: encode}
}

type encodedWriter[T any] struct {
	w      io.Writer
	encode func(io.Writer, T) error
}

func (e *encodedWriter[T]) Write(p []T) (n int, err error) {
	for _, item := range p {
		if err = e.encode(e.w, item); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (e *encodedWriter[T]) WriteItem(item T) error {
	return e.encode(e.w, item)
}
