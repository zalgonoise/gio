package gio

import (
	"errors"
	"io"
	"reflect"
	"sync"
	"unsafe"
)

// Seek whence values.
const (
	SeekStart   = io.SeekStart   // seek relative to the origin of the source
	SeekCurrent = io.SeekCurrent // seek relative to the current offset
	SeekEnd     = io.SeekEnd     // seek relative to the end
)

// EOF is the error returned by Read when no more input is available.
// (Read must return EOF itself, not an error wrapping EOF,
// because callers will test for EOF using ==.)
// Functions should return EOF only to signal a graceful end of input.
// If the EOF occurs unexpectedly in a structured data stream,
// the appropriate error is either [ErrUnexpectedEOF] or some other error
// giving more detail.
var EOF = io.EOF

// ErrShortWrite means that a write accepted fewer T items than requested
// but failed to return an explicit error.
var ErrShortWrite = io.ErrShortWrite

// ErrInvalidWrite means that a write returned an impossible count.
var ErrInvalidWrite = errors.New("invalid write result")

// ErrShortBuffer means that a read required a longer buffer than was provided.
var ErrShortBuffer = io.ErrShortBuffer

// ErrUnexpectedEOF means that EOF was encountered in the
// middle of reading a fixed-size block or data structure.
var ErrUnexpectedEOF = io.ErrUnexpectedEOF

// ErrNoProgress is returned by some clients of a Reader when
// many calls to Read have failed to return any data or error,
// usually the sign of a broken Reader implementation.
var ErrNoProgress = io.ErrNoProgress

// ErrWhence is returned by Seek when whence is invalid.
var ErrWhence = errors.New("Seek: invalid whence")

// ErrOffset is returned by Seek or WriteAt when offset is negative.
var ErrOffset = errors.New("Seek: invalid offset")

// Reader is the interface that wraps the basic Read method.
//
// Read reads up to len(p) T into p. It returns the number of T items
// read (0 <= n <= len(p)) and any error encountered. Even if Read
// returns n < len(p), it may use all of p as scratch space during the call.
// If some data is available but not len(p) T items, Read conventionally
// returns what is available instead of waiting for more.
//
// When Read encounters an error or end-of-file condition after
// successfully reading n > 0 T items, it returns the number of
// T items read. It may return the (non-nil) error from the same call
// or return the error (and n == 0) from a subsequent call.
// An instance of this general case is that a Reader returning
// a non-zero number of T items at the end of the input stream may
// return either err == EOF or err == nil. The next Read should
// return 0, EOF.
//
// Callers should always process the n > 0 T items returned before
// considering the error err. Doing so correctly handles I/O errors
// that happen after reading some T items and also both of the
// allowed EOF behaviors.
//
// If len(p) == 0, Read should always return n == 0. It may return a
// non-nil error if some error condition is known, such as EOF.
//
// Implementations of Read are discouraged from returning a
// zero T item count with a nil error, except when len(p) == 0.
// Callers should treat a return of 0 and nil as indicating that
// nothing happened; in particular it does not indicate EOF.
//
// Implementations must not retain p.
type Reader[T any] interface {
	Read(p []T) (n int, err error)
}

// Writer is the interface that wraps the basic Write method.
//
// Write writes len(p) T items from p to the underlying data stream.
// It returns the number of T items written from p (0 <= n <= len(p))
// and any error encountered that caused the write to stop early.
// Write must return a non-nil error if it returns n < len(p).
// Write must not modify the slice data, even temporarily.
//
// Implementations must not retain p.
type Writer[T any] interface {
	Write(p []T) (n int, err error)
}

// Closer is the interface that wraps the basic Close method.
//
// The behavior of Close after the first call is undefined.
// Specific implementations may document their own behavior.
type Closer = io.Closer

// Seeker is the interface that wraps the basic Seek method.
//
// Seek sets the offset for the next Read or Write to offset,
// interpreted according to whence:
// SeekStart means relative to the start of the source,
// SeekCurrent means relative to the current offset, and
// SeekEnd means relative to the end
// (for example, offset = -2 specifies the penultimate T item of the source).
// Seek returns the new offset relative to the start of the
// source or an error, if any.
//
// Seeking to an offset before the start of the source is an error.
// Seeking to any positive offset may be allowed, but if the new offset exceeds
// the size of the underlying object the behavior of subsequent I/O operations
// is implementation-dependent.
type Seeker = io.Seeker

// ReadWriter is the interface that groups the basic Read and Write methods.
type ReadWriter[T any] interface {
	Reader[T]
	Writer[T]
}

// ReadCloser is the interface that groups the basic Read and Close methods.
type ReadCloser[T any] interface {
	Reader[T]
	Closer
}

// WriteCloser is the interface that groups the basic Write and Close methods.
type WriteCloser[T any] interface {
	Writer[T]
	Closer
}

// ReadWriteCloser is the interface that groups the basic Read, Write and Close methods.
type ReadWriteCloser[T any] interface {
	Reader[T]
	Writer[T]
	Closer
}

// ReadSeeker is the interface that groups the basic Read and Seek methods.
type ReadSeeker[T any] interface {
	Reader[T]
	Seeker
}

// ReadSeekCloser is the interface that groups the basic Read, Seek and Close
// methods.
type ReadSeekCloser[T any] interface {
	Reader[T]
	Seeker
	Closer
}

// WriteSeeker is the interface that groups the basic Write and Seek methods.
type WriteSeeker[T any] interface {
	Writer[T]
	Seeker
}

// ReadWriteSeeker is the interface that groups the basic Read, Write and Seek methods.
type ReadWriteSeeker[T any] interface {
	Reader[T]
	Writer[T]
	Seeker
}

// ReaderFrom is the interface that wraps the ReadFrom method.
//
// ReadFrom reads data from r until EOF or error.
// The return value n is the number of T items read.
// Any error except EOF encountered during the read is also returned.
//
// The Copy function uses ReaderFrom if available.
type ReaderFrom[T any] interface {
	ReadFrom(r Reader[T]) (n int64, err error)
}

// WriterTo is the interface that wraps the WriteTo method.
//
// WriteTo writes data to w until there's no more data to write or
// when an error occurs. The return value n is the number of T items
// written. Any error encountered during the write is also returned.
//
// The Copy function uses WriterTo if available.
type WriterTo[T any] interface {
	WriteTo(w Writer[T]) (n int64, err error)
}

// ReaderAt is the interface that wraps the basic ReadAt method.
//
// ReadAt reads len(p) T items into p starting at offset off in the
// underlying input source. It returns the number of T items
// read (0 <= n <= len(p)) and any error encountered.
//
// When ReadAt returns n < len(p), it returns a non-nil error
// explaining why more T items were not returned. In this respect,
// ReadAt is stricter than Read.
//
// Even if ReadAt returns n < len(p), it may use all of p as scratch
// space during the call. If some data is available but not len(p) T items,
// ReadAt blocks until either all the data is available or an error occurs.
// In this respect ReadAt is different from Read.
//
// If the n = len(p) T items returned by ReadAt are at the end of the
// input source, ReadAt may return either err == EOF or err == nil.
//
// If ReadAt is reading from an input source with a seek offset,
// ReadAt should not affect nor be affected by the underlying
// seek offset.
//
// Clients of ReadAt can execute parallel ReadAt calls on the
// same input source.
//
// Implementations must not retain p.
type ReaderAt[T any] interface {
	ReadAt(p []T, off int64) (n int, err error)
}

// WriterAt is the interface that wraps the basic WriteAt method.
//
// WriteAt writes len(p) T items from p to the underlying data stream
// at offset off. It returns the number of T items written from p (0 <= n <= len(p))
// and any error encountered that caused the write to stop early.
// WriteAt must return a non-nil error if it returns n < len(p).
//
// If WriteAt is writing to a destination with a seek offset,
// WriteAt should not affect nor be affected by the underlying
// seek offset.
//
// Clients of WriteAt can execute parallel WriteAt calls on the same
// destination if the ranges do not overlap.
//
// Implementations must not retain p.
type WriterAt[T any] interface {
	WriteAt(p []T, off int64) (n int, err error)
}

// ByteReader is the interface that wraps the ReadByte method.
type ByteReader = io.ByteReader

// ByteScanner is the interface that adds the UnreadByte method to the
// basic ReadByte method.
type ByteScanner = io.ByteScanner

// ByteWriter is the interface that wraps the WriteByte method.
type ByteWriter = io.ByteWriter

// RuneReader is the interface that wraps the ReadRune method.
type RuneReader = io.RuneReader

// RuneScanner is the interface that adds the UnreadRune method to the
// basic ReadRune method.
type RuneScanner = io.RuneScanner

// StringWriter is the interface that wraps the WriteString method.
type StringWriter = io.StringWriter

// WriteString writes the contents of the string s to w, which accepts a slice of bytes.
// If w implements [StringWriter], [StringWriter.WriteString] is invoked directly.
// Otherwise, [Writer.Write] is called exactly once.
func WriteString(w Writer[byte], s string) (n int, err error) {
	return io.WriteString(w, s)
}

const minBufItems = 64

// defaultBufferSize returns a buffer element count targeting targetBytes of memory
// based on unsafe.Sizeof(T), with a minimum of minBufItems (or targetBytes if smaller).
func defaultBufferSize[T any](targetBytes int) int {
	var zero T
	sz := int(unsafe.Sizeof(zero))
	if sz <= 1 {
		return targetBytes
	}
	n := targetBytes / sz
	floor := min(minBufItems, targetBytes)
	if n < floor {
		return floor
	}
	return n
}

// ReadAtLeast reads from r into buf until it has read at least min T items.
// It returns the number of T items copied and an error if fewer T items were read.
// The error is EOF only if no T items were read.
// If an EOF happens after reading fewer than min T items,
// ReadAtLeast returns ErrUnexpectedEOF.
// If min is greater than the length of buf, ReadAtLeast returns ErrShortBuffer.
// On return, n >= min if and only if err == nil.
// If r returns an error having read at least min T items, the error is dropped.
func ReadAtLeast[T any](r Reader[T], buf []T, min int) (n int, err error) {
	if len(buf) < min {
		return 0, ErrShortBuffer
	}
	for n < min && err == nil {
		var nn int
		nn, err = r.Read(buf[n:])
		n += nn
	}
	if n >= min {
		err = nil
	} else if n > 0 && err == EOF {
		err = ErrUnexpectedEOF
	}
	return
}

// ReadFull reads exactly len(buf) T items from r into buf.
// It returns the number of T items copied and an error if fewer T items were read.
// The error is EOF only if no T items were read.
// If an EOF happens after reading some but not all the T items,
// ReadFull returns ErrUnexpectedEOF.
// On return, n == len(buf) if and only if err == nil.
// If r returns an error having read at least len(buf) T items, the error is dropped.
func ReadFull[T any](r Reader[T], buf []T) (n int, err error) {
	return ReadAtLeast(r, buf, len(buf))
}

// CopyN copies n T items (or until an error) from src to dst.
// It returns the number of T items copied and the earliest
// error encountered while copying.
// On return, written == n if and only if err == nil.
//
// If dst implements the ReaderFrom interface,
// the copy is implemented using it.
func CopyN[T any](dst Writer[T], src Reader[T], n int64) (written int64, err error) {
	written, err = Copy(dst, LimitReader(src, n))
	if written == n {
		return n, nil
	}
	if written < n && err == nil {
		// src stopped early; must have been EOF.
		err = EOF
	}
	return
}

// Copy copies from src to dst until either EOF is reached
// on src or an error occurs. It returns the number of T items
// copied and the first error encountered while copying, if any.
//
// A successful Copy returns err == nil, not err == EOF.
// Because Copy is defined to read from src until EOF, it does
// not treat an EOF from Read as an error to be reported.
//
// If src implements the WriterTo interface,
// the copy is implemented by calling src.WriteTo(dst).
// Otherwise, if dst implements the ReaderFrom interface,
// the copy is implemented by calling dst.ReadFrom(src).
func Copy[T any](dst Writer[T], src Reader[T]) (written int64, err error) {
	return copyBuffer[T](dst, src, nil)
}

// CopyBuffer is identical to Copy except that it stages through the
// provided buffer (if one is required) rather than allocating a
// temporary one. If buf is nil, one is allocated; otherwise if it has
// zero length, CopyBuffer panics.
//
// If either src implements WriterTo or dst implements ReaderFrom,
// buf will not be used to perform the copy.
func CopyBuffer[T any](dst Writer[T], src Reader[T], buf []T) (written int64, err error) {
	if buf != nil && len(buf) == 0 {
		panic("empty buffer in CopyBuffer")
	}
	return copyBuffer(dst, src, buf)
}

// copyBuffer is the actual implementation of Copy and CopyBuffer.
// if buf is nil, one is allocated.
func copyBuffer[T any](dst Writer[T], src Reader[T], buf []T) (written int64, err error) {
	// If the reader has a WriteTo method, use it to do the copy.
	// Avoids an allocation and a copy.
	if wt, ok := src.(WriterTo[T]); ok {
		return wt.WriteTo(dst)
	}
	if wt, ok := any(src).(io.WriterTo); ok {
		if w, ok := any(dst).(io.Writer); ok {
			return wt.WriteTo(w)
		}
	}
	// Similarly, if the writer has a ReadFrom method, use it to do the copy.
	if rf, ok := dst.(ReaderFrom[T]); ok {
		return rf.ReadFrom(src)
	}
	if rf, ok := any(dst).(io.ReaderFrom); ok {
		if r, ok := any(src).(io.Reader); ok {
			return rf.ReadFrom(r)
		}
	}
	if buf == nil {
		size := defaultBufferSize[T](32 * 1024)
		if l, ok := src.(*LimitedReader[T]); ok && int64(size) > l.N {
			size = int(max(l.N, 1))
		}
		buf = make([]T, size)
	}
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = ErrInvalidWrite
				}
			}
			written += int64(nw)
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

// LimitReader returns a Reader that reads from r
// but stops with EOF after n T items.
// The underlying implementation is a *LimitedReader.
func LimitReader[T any](r Reader[T], n int64) Reader[T] { return &LimitedReader[T]{r, n} }

// A LimitedReader reads from R but limits the amount of
// data returned to just N T items. Each call to Read
// updates N to reflect the new amount remaining.
// Read returns EOF when N <= 0 or when the underlying R returns EOF.
type LimitedReader[T any] struct {
	R Reader[T] // underlying reader
	N int64     // max T items remaining
}

func (l *LimitedReader[T]) Read(p []T) (n int, err error) {
	if l.N <= 0 {
		return 0, EOF
	}
	if int64(len(p)) > l.N {
		p = p[0:l.N]
	}
	n, err = l.R.Read(p)
	l.N -= int64(n)
	return
}

// NewSectionReader returns a SectionReader that reads from r
// starting at offset off and stops with EOF after n T items.
func NewSectionReader[T any](r ReaderAt[T], off int64, n int64) *SectionReader[T] {
	var remaining int64
	const maxint64 = 1<<63 - 1
	if off <= maxint64-n {
		remaining = n + off
	} else {
		// Overflow, with no way to return error.
		// Assume we can read up to an offset of 1<<63 - 1.
		remaining = maxint64
	}
	return &SectionReader[T]{r, off, off, remaining, n}
}

// SectionReader implements Read, Seek, and ReadAt on a section
// of an underlying ReaderAt.
type SectionReader[T any] struct {
	r     ReaderAt[T] // constant after creation
	base  int64       // constant after creation
	off   int64
	limit int64 // constant after creation
	n     int64 // constant after creation
}

func (s *SectionReader[T]) Read(p []T) (n int, err error) {
	if s.off >= s.limit {
		return 0, EOF
	}
	if max := s.limit - s.off; int64(len(p)) > max {
		p = p[0:max]
	}
	n, err = s.r.ReadAt(p, s.off)
	s.off += int64(n)
	return
}

func (s *SectionReader[T]) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	default:
		return 0, ErrWhence
	case SeekStart:
		offset += s.base
	case SeekCurrent:
		offset += s.off
	case SeekEnd:
		offset += s.limit
	}
	if offset < s.base {
		return 0, ErrOffset
	}
	s.off = offset
	return offset - s.base, nil
}

func (s *SectionReader[T]) ReadAt(p []T, off int64) (n int, err error) {
	if off < 0 || off >= s.Size() {
		return 0, EOF
	}
	off += s.base
	if max := s.limit - off; int64(len(p)) > max {
		p = p[0:max]
		n, err = s.r.ReadAt(p, off)
		if err == nil {
			err = EOF
		}
		return n, err
	}
	return s.r.ReadAt(p, off)
}

// Size returns the size of the section in T items.
func (s *SectionReader[T]) Size() int64 { return s.limit - s.base }

// Outer returns the underlying [ReaderAt] and offsets for the section.
//
// The returned values are the same that were passed to [NewSectionReader]
// when the [SectionReader] was created.
func (s *SectionReader[T]) Outer() (r ReaderAt[T], off int64, n int64) {
	return s.r, s.base, s.n
}

// An OffsetWriter maps writes at offset base to offset base+off in the underlying writer.
type OffsetWriter[T any] struct {
	w    WriterAt[T]
	base int64 // the original offset
	off  int64 // the current offset
}

// NewOffsetWriter returns an [OffsetWriter] that writes to w
// starting at offset off.
func NewOffsetWriter[T any](w WriterAt[T], off int64) *OffsetWriter[T] {
	return &OffsetWriter[T]{w, off, off}
}

func (o *OffsetWriter[T]) Write(p []T) (n int, err error) {
	n, err = o.w.WriteAt(p, o.off)
	o.off += int64(n)
	return
}

func (o *OffsetWriter[T]) WriteAt(p []T, off int64) (n int, err error) {
	if off < 0 {
		return 0, ErrOffset
	}

	off += o.base
	return o.w.WriteAt(p, off)
}

func (o *OffsetWriter[T]) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	default:
		return 0, ErrWhence
	case SeekStart:
		offset += o.base
	case SeekCurrent:
		offset += o.off
	}
	if offset < o.base {
		return 0, ErrOffset
	}
	o.off = offset
	return offset - o.base, nil
}

// TeeReader returns a Reader that writes to w what it reads from r.
// All reads from r performed through it are matched with
// corresponding writes to w. There is no internal buffering -
// the write must complete before the read completes.
// Any error encountered while writing is reported as a read error.
func TeeReader[T any](r Reader[T], w Writer[T]) Reader[T] {
	return &teeReader[T]{r, w}
}

type teeReader[T any] struct {
	r Reader[T]
	w Writer[T]
}

func (t *teeReader[T]) Read(p []T) (n int, err error) {
	n, err = t.r.Read(p)
	if n > 0 {
		if n, err := t.w.Write(p[:n]); err != nil {
			return n, err
		}
	}
	return
}

// Discard returns a Writer[T] on which all Write, WriteItem, WriteString,
// and ReadFrom calls succeed without doing anything.
func Discard[T any]() Writer[T] {
	return discard[T]{}
}

type discard[T any] struct{}

// discard implements ReaderFrom and ItemWriter as optimizations so Copy and
// WriteItem to Discard can avoid doing unnecessary work.
var (
	_ ReaderFrom[any] = discard[any]{}
	_ ItemWriter[any] = discard[any]{}
	_ StringWriter    = discard[byte]{}
)

func (discard[T]) Write(p []T) (int, error) {
	return len(p), nil
}

func (discard[T]) WriteItem(T) error {
	return nil
}

func (discard[T]) WriteString(s string) (int, error) {
	return len(s), nil
}

var discardPools sync.Map // map[reflect.Type]*sync.Pool

func getDiscardPool[T any]() *sync.Pool {
	rt := reflect.TypeFor[T]()
	if p, ok := discardPools.Load(rt); ok {
		return p.(*sync.Pool)
	}
	size := defaultBufferSize[T](8192)
	pool := &sync.Pool{
		New: func() any {
			b := make([]T, size)
			return &b
		},
	}
	actual, _ := discardPools.LoadOrStore(rt, pool)
	return actual.(*sync.Pool)
}

func (discard[T]) ReadFrom(r Reader[T]) (n int64, err error) {
	pool := getDiscardPool[T]()
	bufp := pool.Get().(*[]T)
	readSize := 0
	for {
		readSize, err = r.Read(*bufp)
		n += int64(readSize)
		if err != nil {
			clear(*bufp)
			pool.Put(bufp)
			if err == EOF {
				return n, nil
			}
			return
		}
	}
}

// NopCloser returns a ReadCloser with a no-op Close method wrapping
// the provided Reader r.
// If r implements WriterTo (or io.WriterTo when T is byte), the returned
// ReadCloser will implement WriterTo by forwarding calls to r.
func NopCloser[T any](r Reader[T]) ReadCloser[T] {
	if _, ok := r.(WriterTo[T]); ok {
		return nopCloserWriterTo[T]{r}
	}
	if _, ok := any(r).(io.WriterTo); ok {
		return nopCloserWriterTo[T]{r}
	}
	return nopCloser[T]{r}
}

type nopCloser[T any] struct {
	Reader[T]
}

func (nopCloser[T]) Close() error { return nil }

type nopCloserWriterTo[T any] struct {
	Reader[T]
}

func (nopCloserWriterTo[T]) Close() error { return nil }

func (c nopCloserWriterTo[T]) WriteTo(w Writer[T]) (n int64, err error) {
	if wt, ok := c.Reader.(WriterTo[T]); ok {
		return wt.WriteTo(w)
	}
	if wt, ok := any(c.Reader).(io.WriterTo); ok {
		if iw, ok := any(w).(io.Writer); ok {
			return wt.WriteTo(iw)
		}
	}
	return copyBuffer(w, c.Reader, nil)
}

// ReadAll reads from r until an error or EOF and returns the data it read.
// A successful call returns err == nil, not err == EOF. Because ReadAll is
// defined to read from src until EOF, it does not treat an EOF from Read
// as an error to be reported.
func ReadAll[T any](r Reader[T]) ([]T, error) {
	initCap := defaultBufferSize[T](512)
	b := make([]T, 0, initCap)
	next := max(initCap/2, 16)
	chunks := make([][]T, 0, 4)
	var finalSize int
	for {
		n, err := r.Read(b[len(b):cap(b)])
		b = b[:len(b)+n]
		if err != nil {
			if err == EOF {
				err = nil
			}
			if len(chunks) == 0 {
				return b, err
			}

			finalSize += len(b)
			final := make([]T, 0, finalSize)
			for _, chunk := range chunks {
				final = append(final, chunk...)
			}
			final = append(final, b...)
			return final, err
		}

		if cap(b)-len(b) < max(cap(b)/16, 1) {
			chunks = append(chunks, b)
			finalSize += len(b)
			b = make([]T, 0, next)
			next += next / 2
		}
	}
}
