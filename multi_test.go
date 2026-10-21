package gio_test

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zalgonoise/gio/v2"
)

type byteAndEOFReader byte

func (b byteAndEOFReader) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		panic("unexpected call")
	}
	p[0] = byte(b)
	return 1, gio.EOF
}

func callDepth(callers []uintptr) (depth int) {
	frames := runtime.CallersFrames(callers)
	more := true
	for more {
		_, more = frames.Next()
		depth++
	}
	return
}

func TestMultiReader(t *testing.T) {
	for _, testcase := range [...]struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "SequentialReads",
			fn: func(t *testing.T) {
				var mr gio.Reader[byte]
				var buf []byte
				withFooBar := func(tests func()) {
					r1 := strings.NewReader("foo ")
					r2 := strings.NewReader("")
					r3 := strings.NewReader("bar")
					mr = gio.MultiReader[byte](r1, r2, r3)
					buf = make([]byte, 20)
					tests()
				}
				expectRead := func(size int, expected string, eerr error) {
					n, gerr := mr.Read(buf[0:size])
					require.Equal(t, len(expected), n)
					require.Equal(t, expected, string(buf[0:n]))
					require.Equal(t, eerr, gerr)
					buf = buf[n:]
				}
				withFooBar(func() {
					expectRead(2, "fo", nil)
					expectRead(5, "o ", nil)
					expectRead(5, "bar", nil)
					expectRead(5, "", gio.EOF)
				})
				withFooBar(func() {
					expectRead(4, "foo ", nil)
					expectRead(1, "b", nil)
					expectRead(3, "ar", nil)
					expectRead(1, "", gio.EOF)
				})
				withFooBar(func() {
					expectRead(5, "foo ", nil)
				})
			},
		},
		{
			name: "AsWriterToAndResumeOnError",
			fn: func(t *testing.T) {
				mr := gio.MultiReader[byte](
					strings.NewReader("foo "),
					gio.MultiReader[byte](
						strings.NewReader(""),
						strings.NewReader("bar"),
					),
				)
				mrAsWriterTo, ok := mr.(gio.WriterTo[byte])
				require.True(t, ok)

				sink := &strings.Builder{}
				n, err := mrAsWriterTo.WriteTo(sink)
				require.NoError(t, err)
				require.Equal(t, int64(7), n)
				require.Equal(t, "foo bar", sink.String())

				// Test error mid-WriteTo
				wantErr := errors.New("mid error")
				mrErr := gio.MultiReader[int](
					gio.NewReader([]int{1, 2}),
					zeroErrReader[int]{err: wantErr},
					gio.NewReader([]int{3, 4}),
				)
				dst := gio.NewSliceWriter[int](nil)
				n, err = mrErr.(gio.WriterTo[int]).WriteTo(dst)
				require.ErrorIs(t, err, wantErr)
				require.Equal(t, int64(3), n)
			},
		},
		{
			name: "InputSliceCopyIsolation",
			fn: func(t *testing.T) {
				slice := []gio.Reader[byte]{strings.NewReader("hello world")}
				r := gio.MultiReader(slice...)
				slice[0] = nil
				data, err := gio.ReadAll(r)
				require.NoError(t, err)
				require.Equal(t, "hello world", string(data))
			},
		},
		{
			name: "FlattenChainedMultiReaders",
			fn: func(t *testing.T) {
				pc := make([]uintptr, 1000)
				n := runtime.Callers(0, pc)
				myDepth := callDepth(pc[:n])
				var readDepth int
				var r gio.Reader[byte] = gio.MultiReader[byte](gio.ReaderFunc[byte](func(p []byte) (int, error) {
					n := runtime.Callers(1, pc)
					readDepth = callDepth(pc[:n])
					return 0, errors.New("irrelevant")
				}))

				for i := 0; i < 100; i++ {
					r = gio.MultiReader(r)
				}
				_, _ = r.Read(nil)
				require.Equal(t, myDepth+2, readDepth)
			},
		},
		{
			name: "SingleByteWithEOFAndFinalEOF",
			fn: func(t *testing.T) {
				got, err := gio.ReadAll(gio.LimitReader(gio.MultiReader[byte](byteAndEOFReader('a'), byteAndEOFReader('b')), 10))
				require.NoError(t, err)
				require.Equal(t, "ab", string(got))

				r := gio.MultiReader[byte](bytes.NewReader(nil), byteAndEOFReader('a'))
				buf := make([]byte, 2)
				n, err := r.Read(buf)
				require.ErrorIs(t, err, gio.EOF)
				require.Equal(t, 1, n)
			},
		},
		{
			name: "FreesExhaustedReaders",
			fn: func(t *testing.T) {
				var mr gio.Reader[byte]
				closed := make(chan struct{})
				func() {
					buf1 := bytes.NewReader([]byte("foo"))
					buf2 := bytes.NewReader([]byte("bar"))
					mr = gio.MultiReader[byte](buf1, buf2)
					runtime.SetFinalizer(buf1, func(*bytes.Reader) {
						close(closed)
					})
				}()

				buf := make([]byte, 4)
				n, err := gio.ReadFull(mr, buf)
				require.NoError(t, err)
				require.Equal(t, 4, n)
				require.Equal(t, "foob", string(buf))

				runtime.GC()
				select {
				case <-closed:
				case <-time.After(5 * time.Second):
					t.Fatal("timeout waiting for collection of buf1")
				}

				n, err = gio.ReadFull(mr, buf[:2])
				require.NoError(t, err)
				require.Equal(t, 2, n)
				require.Equal(t, "ar", string(buf[:2]))
			},
		},
		{
			name: "InterleavedMultiReader",
			fn: func(t *testing.T) {
				r1 := strings.NewReader("123")
				r2 := strings.NewReader("45678")

				mr1 := gio.MultiReader[byte](r1, r2)
				mr2 := gio.MultiReader[byte](mr1)

				buf := make([]byte, 4)
				n, err := gio.ReadFull(mr2, buf)
				require.NoError(t, err)
				require.Equal(t, "1234", string(buf[:n]))

				n, err = gio.ReadFull(mr1, buf)
				require.NoError(t, err)
				require.Equal(t, "5678", string(buf[:n]))
			},
		},
	} {
		t.Run(testcase.name, testcase.fn)
	}
}

func testMultiWriter(t *testing.T, sink interface {
	gio.Writer[byte]
	fmt.Stringer
}) {
	h := sha1.New()
	mw := gio.MultiWriter[byte](h, sink)

	sourceString := "My input text."
	source := strings.NewReader(sourceString)
	written, err := gio.Copy[byte](mw, source)

	require.NoError(t, err)
	require.Equal(t, int64(len(sourceString)), written)
	require.Equal(t, "01cb303fa8c30a64123067c5aa6284ba7ec2d31b", fmt.Sprintf("%x", h.Sum(nil)))
	require.Equal(t, sourceString, sink.String())
}

type shortStringWriter struct{}

func (shortStringWriter) Write(p []byte) (int, error)       { return len(p), nil }
func (shortStringWriter) WriteString(s string) (int, error) { return len(s) - 1, nil }

func TestMultiWriter(t *testing.T) {
	for _, testcase := range [...]struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "WithoutStringWriter",
			fn: func(t *testing.T) {
				sink := new(bytes.Buffer)
				testMultiWriter(t, struct {
					gio.Writer[byte]
					fmt.Stringer
				}{sink, sink})
			},
		},
		{
			name: "WithStringWriter",
			fn: func(t *testing.T) {
				testMultiWriter(t, new(bytes.Buffer))
			},
		},
		{
			name: "WriteStringZeroAllocAndSingleAlloc",
			fn: func(t *testing.T) {
				var sink1, sink2 bytes.Buffer
				mwZero := gio.MultiWriter[byte](&sink1, &sink2)
				allocsZero := int(testing.AllocsPerRun(1000, func() {
					_, _ = io.WriteString(mwZero, "foo")
				}))
				require.Zero(t, allocsZero)

				type simpleWriter struct {
					gio.Writer[byte]
				}
				mwOne := gio.MultiWriter[byte](simpleWriter{&sink1}, simpleWriter{&sink2})
				allocsOne := int(testing.AllocsPerRun(1000, func() {
					_, _ = io.WriteString(mwOne, "foo")
				}))
				require.Equal(t, 1, allocsOne)
			},
		},
		{
			name: "WriteStringErrors",
			fn: func(t *testing.T) {
				mwShort := gio.MultiWriter[byte](shortStringWriter{})
				_, err := gio.WriteString(mwShort, "hello")
				require.ErrorIs(t, err, gio.ErrShortWrite)

				wantErr := errors.New("write error")
				mwErr := gio.MultiWriter[byte](errWriter[byte]{err: wantErr})
				_, err = gio.WriteString(mwErr, "hello")
				require.ErrorIs(t, err, wantErr)

				// Non-byte MultiWriter calling WriteString returns ErrInvalidWrite
				mwInt := gio.MultiWriter[int](gio.NewSliceWriter[int](nil))
				_, err = mwInt.(gio.StringWriter).WriteString("hello")
				require.ErrorIs(t, err, gio.ErrInvalidWrite)
			},
		},
		{
			name: "WriteItemFanOut",
			fn: func(t *testing.T) {
				w1 := gio.NewSliceWriter[int](nil)
				w2 := gio.NewSliceWriter[int](nil)
				mw := gio.MultiWriter(w1, w2)

				require.NoError(t, gio.WriteItem(mw, 77))
				require.Equal(t, []int{77}, w1.Slice())
				require.Equal(t, []int{77}, w2.Slice())

				wantErr := errors.New("item fail")
				mwErr := gio.MultiWriter[int](w1, errWriter[int]{err: wantErr})
				require.ErrorIs(t, gio.WriteItem(mwErr, 88), wantErr)
			},
		},
		{
			name: "SingleChainFlatten",
			fn: func(t *testing.T) {
				pc := make([]uintptr, 1000)
				n := runtime.Callers(0, pc)
				myDepth := callDepth(pc[:n])
				var writeDepth int
				var w gio.Writer[byte] = gio.MultiWriter[byte](gio.WriterFunc[byte](func(p []byte) (int, error) {
					n := runtime.Callers(1, pc)
					writeDepth += callDepth(pc[:n])
					return 0, nil
				}))

				mw := w
				for i := 0; i < 100; i++ {
					mw = gio.MultiWriter(w)
				}

				mw = gio.MultiWriter(w, mw, w, mw)
				_, _ = mw.Write(nil)
				require.Equal(t, 4*(myDepth+2), writeDepth)
			},
		},
		{
			name: "WriteErrorAndShortWrite",
			fn: func(t *testing.T) {
				f1 := gio.WriterFunc[byte](func(p []byte) (int, error) {
					return len(p) / 2, gio.ErrShortWrite
				})
				f2 := gio.WriterFunc[byte](func(p []byte) (int, error) {
					t.Errorf("MultiWriter called f2.Write")
					return len(p), nil
				})
				w := gio.MultiWriter[byte](f1, f2)
				n, err := w.Write(make([]byte, 100))
				require.ErrorIs(t, err, gio.ErrShortWrite)
				require.Equal(t, 50, n)

				wShort := gio.MultiWriter[int](shortWriter[int]{})
				_, err = wShort.Write([]int{1, 2, 3})
				require.ErrorIs(t, err, gio.ErrShortWrite)
			},
		},
		{
			name: "InputSliceCopyIsolation",
			fn: func(t *testing.T) {
				var buf bytes.Buffer
				slice := []gio.Writer[byte]{&buf}
				w := gio.MultiWriter(slice...)
				slice[0] = nil
				n, err := w.Write([]byte("hello world"))
				require.NoError(t, err)
				require.Equal(t, 11, n)
				require.Equal(t, "hello world", buf.String())
			},
		},
	} {
		t.Run(testcase.name, testcase.fn)
	}
}
