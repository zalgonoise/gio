package gio_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zalgonoise/gio"
)

// A version of bytes.Buffer without ReadFrom and WriteTo
type Buffer struct {
	bytes.Buffer
	gio.ReaderFrom[byte] // conflicts with and hides bytes.Buffer's ReaderFrom.
	gio.WriterTo[byte]   // conflicts with and hides bytes.Buffer's WriterTo.
}

type writeToChecker struct {
	bytes.Buffer
	writeToCalled bool
}

func (wt *writeToChecker) WriteTo(w gio.Writer[byte]) (int64, error) {
	wt.writeToCalled = true
	return wt.Buffer.WriteTo(w)
}

type zeroErrReader[T any] struct {
	err error
}

func (r zeroErrReader[T]) Read(p []T) (int, error) {
	var zero T
	return copy(p, []T{zero}), r.err
}

type errWriter[T any] struct {
	err error
}

func (w errWriter[T]) Write([]T) (int, error) {
	return 0, w.err
}

type noReadFrom[T any] struct {
	w gio.Writer[T]
}

func (w *noReadFrom[T]) Write(p []T) (n int, err error) {
	return w.w.Write(p)
}

type wantedAndErrReader[T any] struct{}

func (wantedAndErrReader[T]) Read(p []T) (int, error) {
	return len(p), errors.New("wantedAndErrReader error")
}

type dataAndErrorBuffer struct {
	err error
	bytes.Buffer
}

func (r *dataAndErrorBuffer) Read(p []byte) (n int, err error) {
	n, err = r.Buffer.Read(p)
	if n > 0 && r.Buffer.Len() == 0 && err == nil {
		err = r.err
	}
	return
}

type largeWriter struct {
	err error
}

func (w largeWriter) Write(p []byte) (int, error) {
	return len(p) + 1, w.err
}

type shortWriter[T any] struct{}

func (shortWriter[T]) Write(p []T) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

func TestCopy(t *testing.T) {
	for _, testcase := range [...]struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "BasicByteCopy",
			fn: func(t *testing.T) {
				rb := new(Buffer)
				wb := new(Buffer)
				rb.WriteString("hello, world.")
				n, err := gio.Copy[byte](wb, rb)
				require.NoError(t, err)
				require.Equal(t, int64(13), n)
				require.Equal(t, "hello, world.", wb.String())
			},
		},
		{
			name: "NegativeLimit",
			fn: func(t *testing.T) {
				rb := new(Buffer)
				wb := new(Buffer)
				rb.WriteString("hello")
				n, err := gio.Copy[byte](wb, &gio.LimitedReader[byte]{R: rb, N: -1})
				require.NoError(t, err)
				require.Zero(t, n)
				require.Empty(t, wb.String())

				n, err = gio.CopyN[byte](wb, rb, -1)
				require.NoError(t, err)
				require.Zero(t, n)
				require.Empty(t, wb.String())
			},
		},
		{
			name: "CopyBufferTiny",
			fn: func(t *testing.T) {
				rb := new(Buffer)
				wb := new(Buffer)
				rb.WriteString("hello, world.")
				n, err := gio.CopyBuffer[byte](wb, rb, make([]byte, 1))
				require.NoError(t, err)
				require.Equal(t, int64(13), n)
				require.Equal(t, "hello, world.", wb.String())
			},
		},
		{
			name: "CopyBufferNil",
			fn: func(t *testing.T) {
				rb := new(Buffer)
				wb := new(Buffer)
				rb.WriteString("hello, world.")
				n, err := gio.CopyBuffer[byte](wb, rb, nil)
				require.NoError(t, err)
				require.Equal(t, int64(13), n)
				require.Equal(t, "hello, world.", wb.String())
			},
		},
		{
			name: "CopyBufferEmptyPanics",
			fn: func(t *testing.T) {
				rb := new(Buffer)
				wb := new(Buffer)
				require.Panics(t, func() {
					_, _ = gio.CopyBuffer[byte](wb, rb, []byte{})
				})
			},
		},
		{
			name: "StdlibReadFrom",
			fn: func(t *testing.T) {
				rb := new(Buffer)
				wb := new(bytes.Buffer) // implements io.ReaderFrom
				rb.WriteString("hello, world.")
				n, err := gio.Copy[byte](wb, rb)
				require.NoError(t, err)
				require.Equal(t, int64(13), n)
				require.Equal(t, "hello, world.", wb.String())
			},
		},
		{
			name: "GenericReaderFrom",
			fn: func(t *testing.T) {
				src := gio.NewReader([]int{1, 2, 3, 4, 5})
				// Hide WriterTo on src so dst.ReadFrom is exercised
				onlyR := struct{ gio.Reader[int] }{src}
				dst := gio.NewSliceWriter[int](nil)
				n, err := gio.Copy(dst, onlyR)
				require.NoError(t, err)
				require.Equal(t, int64(5), n)
				require.Equal(t, []int{1, 2, 3, 4, 5}, dst.Slice())
			},
		},
		{
			name: "StdlibWriteTo",
			fn: func(t *testing.T) {
				rb := new(bytes.Buffer) // implements io.WriterTo
				wb := new(Buffer)
				rb.WriteString("hello, world.")
				n, err := gio.Copy[byte](wb, rb)
				require.NoError(t, err)
				require.Equal(t, int64(13), n)
				require.Equal(t, "hello, world.", wb.String())
			},
		},
		{
			name: "WriteToPriorityOverReadFrom",
			fn: func(t *testing.T) {
				rb := new(writeToChecker)
				wb := new(bytes.Buffer)
				rb.WriteString("hello, world.")
				n, err := gio.Copy[byte](wb, rb)
				require.NoError(t, err)
				require.Equal(t, int64(13), n)
				require.Equal(t, "hello, world.", wb.String())
				require.True(t, rb.writeToCalled)
			},
		},
		{
			name: "ReadErrWriteErr",
			fn: func(t *testing.T) {
				er, ew := errors.New("readError"), errors.New("writeError")
				r, w := zeroErrReader[byte]{err: er}, errWriter[byte]{err: ew}
				n, err := gio.Copy[byte](w, r)
				require.Zero(t, n)
				require.ErrorIs(t, err, ew)
			},
		},
		{
			name: "ShortWriteError",
			fn: func(t *testing.T) {
				r := struct{ gio.Reader[int] }{gio.NewReader([]int{1, 2, 3})}
				w := shortWriter[int]{}
				n, err := gio.Copy(w, r)
				require.Equal(t, int64(2), n)
				require.ErrorIs(t, err, gio.ErrShortWrite)
			},
		},
		{
			name: "LargeWriterInvalidWrite",
			fn: func(t *testing.T) {
				rb := new(Buffer)
				wb := largeWriter{}
				rb.WriteString("hello, world.")
				_, err := gio.Copy[byte](wb, rb)
				require.ErrorIs(t, err, gio.ErrInvalidWrite)

				want := errors.New("largeWriterError")
				rb = new(Buffer)
				wb = largeWriter{err: want}
				rb.WriteString("hello, world.")
				_, err = gio.Copy[byte](wb, rb)
				require.ErrorIs(t, err, want)
			},
		},
		{
			name: "LargeStructCopyBoundedBuffer",
			fn: func(t *testing.T) {
				type block [1024]byte
				items := make([]block, 150)
				items[0][0] = 42
				items[149][1023] = 99
				r := struct{ gio.Reader[block] }{gio.NewReader(items)}
				var out []block
				w := gio.WriterFunc[block](func(p []block) (int, error) {
					out = append(out, p...)
					return len(p), nil
				})
				n, err := gio.Copy(w, r)
				require.NoError(t, err)
				require.Equal(t, int64(150), n)
				require.Equal(t, items, out)
			},
		},
	} {
		t.Run(testcase.name, testcase.fn)
	}
}

func TestCopyN(t *testing.T) {
	for _, testcase := range [...]struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "BasicCopyN",
			fn: func(t *testing.T) {
				rb := new(Buffer)
				wb := new(Buffer)
				rb.WriteString("hello, world.")
				n, err := gio.CopyN[byte](wb, rb, 5)
				require.NoError(t, err)
				require.Equal(t, int64(5), n)
				require.Equal(t, "hello", wb.String())
			},
		},
		{
			name: "CopyNReadFrom",
			fn: func(t *testing.T) {
				rb := new(Buffer)
				wb := new(bytes.Buffer)
				rb.WriteString("hello")
				n, err := gio.CopyN[byte](wb, rb, 5)
				require.NoError(t, err)
				require.Equal(t, int64(5), n)
				require.Equal(t, "hello", wb.String())
			},
		},
		{
			name: "CopyNWriteTo",
			fn: func(t *testing.T) {
				rb := new(bytes.Buffer)
				wb := new(Buffer)
				rb.WriteString("hello, world.")
				n, err := gio.CopyN[byte](wb, rb, 5)
				require.NoError(t, err)
				require.Equal(t, int64(5), n)
				require.Equal(t, "hello", wb.String())
			},
		},
		{
			name: "CopyNEOFBehavior",
			fn: func(t *testing.T) {
				b := new(bytes.Buffer)

				n, err := gio.CopyN[byte](&noReadFrom[byte]{b}, strings.NewReader("foo"), 3)
				require.NoError(t, err)
				require.Equal(t, int64(3), n)

				n, err = gio.CopyN[byte](&noReadFrom[byte]{b}, strings.NewReader("foo"), 4)
				require.ErrorIs(t, err, gio.EOF)
				require.Equal(t, int64(3), n)

				n, err = gio.CopyN[byte](b, strings.NewReader("foo"), 3)
				require.NoError(t, err)
				require.Equal(t, int64(3), n)

				n, err = gio.CopyN[byte](b, strings.NewReader("foo"), 4)
				require.ErrorIs(t, err, gio.EOF)
				require.Equal(t, int64(3), n)

				n, err = gio.CopyN[byte](b, wantedAndErrReader[byte]{}, 5)
				require.NoError(t, err)
				require.Equal(t, int64(5), n)

				n, err = gio.CopyN[byte](&noReadFrom[byte]{b}, wantedAndErrReader[byte]{}, 5)
				require.NoError(t, err)
				require.Equal(t, int64(5), n)
			},
		},
	} {
		t.Run(testcase.name, testcase.fn)
	}
}

func TestReadAtLeastAndReadFull(t *testing.T) {
	for _, testcase := range [...]struct {
		name string
		rb   func() gio.ReadWriter[byte]
	}{
		{
			name: "StandardBuffer",
			rb:   func() gio.ReadWriter[byte] { return new(bytes.Buffer) },
		},
		{
			name: "DataAndEOF",
			rb:   func() gio.ReadWriter[byte] { return &dataAndErrorBuffer{err: gio.EOF} },
		},
		{
			name: "DataAndFakeError",
			rb:   func() gio.ReadWriter[byte] { return &dataAndErrorBuffer{err: fmt.Errorf("fake error")} },
		},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			rb := testcase.rb()
			_, err := rb.Write([]byte("0123"))
			require.NoError(t, err)

			buf := make([]byte, 2)
			n, err := gio.ReadAtLeast[byte](rb, buf, 2)
			require.NoError(t, err)
			require.Equal(t, 2, n)

			n, err = gio.ReadAtLeast[byte](rb, buf, 4)
			require.ErrorIs(t, err, gio.ErrShortBuffer)
			require.Zero(t, n)

			n, err = gio.ReadAtLeast[byte](rb, buf, 1)
			require.NoError(t, err)
			require.Equal(t, 2, n)

			n, err = gio.ReadAtLeast[byte](rb, buf, 2)
			require.ErrorIs(t, err, gio.EOF)
			require.Zero(t, n)

			_, err = rb.Write([]byte("4"))
			require.NoError(t, err)
			n, err = gio.ReadFull[byte](rb, buf)
			want := gio.ErrUnexpectedEOF
			if db, ok := rb.(*dataAndErrorBuffer); ok && db.err != gio.EOF {
				want = db.err
			}
			require.ErrorIs(t, err, want)
			require.Equal(t, 1, n)
		})
	}
}

func TestTeeReader(t *testing.T) {
	src := []byte("hello, world")
	dst := make([]byte, len(src))
	rb := bytes.NewBuffer(src)
	wb := new(bytes.Buffer)
	r := gio.TeeReader[byte](rb, wb)

	n, err := gio.ReadFull(r, dst)
	require.NoError(t, err)
	require.Equal(t, len(src), n)
	require.Equal(t, src, dst)
	require.Equal(t, src, wb.Bytes())

	n, err = r.Read(dst)
	require.ErrorIs(t, err, gio.EOF)
	require.Zero(t, n)

	rb = bytes.NewBuffer(src)
	pr, pw := gio.Pipe[byte]()
	require.NoError(t, pr.Close())
	r = gio.TeeReader[byte](rb, pw)
	n, err = gio.ReadFull(r, dst)
	require.ErrorIs(t, err, gio.ErrClosedPipe)
	require.Zero(t, n)
}

func TestSectionReader(t *testing.T) {
	dat := "a long sample data, 1234567890"
	for _, testcase := range [...]struct {
		name   string
		data   string
		off    int
		n      int
		bufLen int
		at     int
		exp    string
		err    error
	}{
		{name: "EmptySource", data: "", off: 0, n: 10, bufLen: 2, at: 0, exp: "", err: gio.EOF},
		{name: "ZeroBufLen", data: dat, off: 0, n: len(dat), bufLen: 0, at: 0, exp: "", err: nil},
		{name: "OffsetAtEnd", data: dat, off: len(dat), n: 1, bufLen: 1, at: 0, exp: "", err: gio.EOF},
		{name: "LengthExceedsData", data: dat, off: 0, n: len(dat) + 2, bufLen: len(dat), at: 0, exp: dat, err: nil},
		{name: "HalfReadAtZero", data: dat, off: 0, n: len(dat), bufLen: len(dat) / 2, at: 0, exp: dat[:len(dat)/2], err: nil},
		{name: "FullReadAtZero", data: dat, off: 0, n: len(dat), bufLen: len(dat), at: 0, exp: dat, err: nil},
		{name: "HalfReadAtTwo", data: dat, off: 0, n: len(dat), bufLen: len(dat) / 2, at: 2, exp: dat[2 : 2+len(dat)/2], err: nil},
		{name: "OffsetThreeAtTwo", data: dat, off: 3, n: len(dat), bufLen: len(dat) / 2, at: 2, exp: dat[5 : 5+len(dat)/2], err: nil},
		{name: "ShortSectionWithinLimit", data: dat, off: 3, n: len(dat) / 2, bufLen: len(dat)/2 - 2, at: 2, exp: dat[5 : 5+len(dat)/2-2], err: nil},
		{name: "ShortSectionExceedingLimit", data: dat, off: 3, n: len(dat) / 2, bufLen: len(dat)/2 + 2, at: 2, exp: dat[5 : 5+len(dat)/2-2], err: gio.EOF},
		{name: "NegativeAt", data: dat, off: 0, n: 0, bufLen: 0, at: -1, exp: "", err: gio.EOF},
		{name: "PositiveAtOnEmptySection", data: dat, off: 0, n: 0, bufLen: 0, at: 1, exp: "", err: gio.EOF},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			r := strings.NewReader(testcase.data)
			s := gio.NewSectionReader[byte](r, int64(testcase.off), int64(testcase.n))
			buf := make([]byte, testcase.bufLen)
			n, err := s.ReadAt(buf, int64(testcase.at))
			require.Equal(t, len(testcase.exp), n)
			require.Equal(t, testcase.exp, string(buf[:n]))
			require.Equal(t, testcase.err, err)

			outerR, outerOff, outerN := s.Outer()
			require.Equal(t, r, outerR)
			require.Equal(t, int64(testcase.off), outerOff)
			require.Equal(t, int64(testcase.n), outerN)
		})
	}

	t.Run("Seek", func(t *testing.T) {
		br := bytes.NewReader([]byte("foo"))
		sr := gio.NewSectionReader[byte](br, 0, int64(len("foo")))

		for _, whence := range []int{gio.SeekStart, gio.SeekCurrent, gio.SeekEnd, 99} {
			for offset := int64(-3); offset <= 4; offset++ {
				brOff, brErr := br.Seek(offset, whence)
				srOff, srErr := sr.Seek(offset, whence)
				require.Equal(t, brErr != nil, srErr != nil)
				require.Equal(t, brOff, srOff)
			}
		}

		got, err := sr.Seek(100, gio.SeekStart)
		require.NoError(t, err)
		require.Equal(t, int64(100), got)

		n, err := sr.Read(make([]byte, 10))
		require.ErrorIs(t, err, gio.EOF)
		require.Zero(t, n)
	})

	t.Run("SizeAndMaxOverflow", func(t *testing.T) {
		r := strings.NewReader("abcdef")
		const maxint64 = 1<<63 - 1
		sr := gio.NewSectionReader[byte](r, 3, maxint64)
		require.Equal(t, int64(maxint64-3), sr.Size())
		_, off, n := sr.Outer()
		require.Equal(t, int64(3), off)
		require.Equal(t, int64(maxint64), n)

		readN, err := sr.Read(make([]byte, 3))
		require.NoError(t, err)
		require.Equal(t, 3, readN)

		readN, err = sr.Read(make([]byte, 3))
		require.ErrorIs(t, err, gio.EOF)
		require.Zero(t, readN)
	})
}

func TestOffsetWriter(t *testing.T) {
	t.Run("WriteAndWriteAt", func(t *testing.T) {
		sw := gio.NewSliceWriter[int](make([]int, 10))
		ow := gio.NewOffsetWriter[int](sw, 3)

		n, err := ow.Write([]int{10, 20, 30})
		require.NoError(t, err)
		require.Equal(t, 3, n)
		require.Equal(t, []int{0, 0, 0, 10, 20, 30, 0, 0, 0, 0}, sw.Slice())

		n, err = ow.WriteAt([]int{99, 88}, 1)
		require.NoError(t, err)
		require.Equal(t, 2, n)
		require.Equal(t, []int{0, 0, 0, 10, 99, 88, 0, 0, 0, 0}, sw.Slice())

		_, err = ow.WriteAt([]int{1}, -1)
		require.ErrorIs(t, err, gio.ErrOffset)
	})

	t.Run("Seek", func(t *testing.T) {
		sw := gio.NewSliceWriter[byte](nil)
		ow := gio.NewOffsetWriter[byte](sw, 2)

		pos, err := ow.Seek(3, gio.SeekStart)
		require.NoError(t, err)
		require.Equal(t, int64(3), pos)

		pos, err = ow.Seek(-1, gio.SeekCurrent)
		require.NoError(t, err)
		require.Equal(t, int64(2), pos)

		_, err = ow.Seek(-1, gio.SeekStart)
		require.ErrorIs(t, err, gio.ErrOffset)

		_, err = ow.Seek(0, gio.SeekEnd)
		require.ErrorIs(t, err, gio.ErrWhence)

		n, err := ow.Write([]byte("ab"))
		require.NoError(t, err)
		require.Equal(t, 2, n)
		require.Equal(t, []byte{0, 0, 0, 0, 'a', 'b'}, sw.Slice())
	})
}

func TestDiscard(t *testing.T) {
	t.Run("WriteAndHelpers", func(t *testing.T) {
		d := gio.Discard[string]()
		n, err := d.Write([]string{"a", "b", "c"})
		require.NoError(t, err)
		require.Equal(t, 3, n)

		require.NoError(t, gio.WriteItem(d, "d"))

		db := gio.Discard[byte]()
		ns, err := gio.WriteString(db, "hello")
		require.NoError(t, err)
		require.Equal(t, 5, ns)
	})

	t.Run("ReadFromMultiTypeConcurrent", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(3)

		go func() {
			defer wg.Done()
			r := struct{ gio.Reader[byte] }{strings.NewReader("hello discard")}
			n, err := gio.Copy(gio.Discard[byte](), r)
			require.NoError(t, err)
			require.Equal(t, int64(13), n)
		}()

		go func() {
			defer wg.Done()
			r := struct{ gio.Reader[int] }{gio.NewReader([]int{1, 2, 3, 4, 5})}
			n, err := gio.Copy(gio.Discard[int](), r)
			require.NoError(t, err)
			require.Equal(t, int64(5), n)
		}()

		go func() {
			defer wg.Done()
			type item struct {
				name string
				vals []int
			}
			r := struct{ gio.Reader[item] }{gio.NewReader([]item{
				{name: "one", vals: []int{1}},
				{name: "two", vals: []int{2}},
			})}
			n, err := gio.Copy(gio.Discard[item](), r)
			require.NoError(t, err)
			require.Equal(t, int64(2), n)
		}()

		wg.Wait()
	})

	t.Run("ReadFromError", func(t *testing.T) {
		wantErr := errors.New("read failure")
		r := zeroErrReader[int]{err: wantErr}
		n, err := gio.Copy(gio.Discard[int](), r)
		require.ErrorIs(t, err, wantErr)
		require.Equal(t, int64(1), n)
	})
}

func TestNopCloser(t *testing.T) {
	for _, testcase := range [...]struct {
		name         string
		r            gio.Reader[byte]
		wantWriterTo bool
	}{
		{
			name:         "NotAWriterTo",
			r:            gio.Reader[byte](nil),
			wantWriterTo: false,
		},
		{
			name: "GenericWriterTo",
			r: struct {
				gio.Reader[byte]
				gio.WriterTo[byte]
			}{
				Reader:   strings.NewReader("abc"),
				WriterTo: gio.NewReader([]byte("abc")),
			},
			wantWriterTo: true,
		},
		{
			name:         "StdlibWriterTo",
			r:            strings.NewReader("stdlib writerTo"),
			wantWriterTo: true,
		},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			nc := gio.NopCloser(testcase.r)
			require.NoError(t, nc.Close())

			wt, got := nc.(gio.WriterTo[byte])
			require.Equal(t, testcase.wantWriterTo, got)
			if got && testcase.r != nil {
				var dst bytes.Buffer
				n, err := wt.WriteTo(&dst)
				require.NoError(t, err)
				require.Positive(t, n)
			}
		})
	}
}

func TestReadAll(t *testing.T) {
	for _, testcase := range [...]struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "SmallByteStream",
			fn: func(t *testing.T) {
				r := strings.NewReader("hello world")
				b, err := gio.ReadAll[byte](r)
				require.NoError(t, err)
				require.Equal(t, "hello world", string(b))
			},
		},
		{
			name: "LargeMultiChunkIntStream",
			fn: func(t *testing.T) {
				input := make([]int, 5000)
				for i := range input {
					input[i] = i * 3
				}
				out, err := gio.ReadAll(gio.NewReader(input))
				require.NoError(t, err)
				require.Equal(t, input, out)
			},
		},
		{
			name: "ErrorMidStream",
			fn: func(t *testing.T) {
				wantErr := errors.New("boom")
				r := zeroErrReader[string]{err: wantErr}
				out, err := gio.ReadAll(r)
				require.ErrorIs(t, err, wantErr)
				require.Equal(t, []string{""}, out)
			},
		},
	} {
		t.Run(testcase.name, testcase.fn)
	}
}

func TestWriteString(t *testing.T) {
	t.Run("WithStringWriter", func(t *testing.T) {
		var buf bytes.Buffer
		n, err := gio.WriteString(&buf, "hello")
		require.NoError(t, err)
		require.Equal(t, 5, n)
		require.Equal(t, "hello", buf.String())
	})

	t.Run("WithoutStringWriter", func(t *testing.T) {
		var buf bytes.Buffer
		w := struct{ gio.Writer[byte] }{&buf}
		n, err := gio.WriteString(w, "world")
		require.NoError(t, err)
		require.Equal(t, 5, n)
		require.Equal(t, "world", buf.String())
	})
}

func BenchmarkCopyNSmall(b *testing.B) {
	bs := bytes.Repeat([]byte{0}, 512+1)
	rd := bytes.NewReader(bs)
	buf := new(Buffer)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = gio.CopyN[byte](buf, rd, 512)
		rd.Reset(bs)
	}
}

func BenchmarkCopyNLarge(b *testing.B) {
	bs := bytes.Repeat([]byte{0}, (32*1024)+1)
	rd := bytes.NewReader(bs)
	buf := new(Buffer)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = gio.CopyN[byte](buf, rd, 32*1024)
		rd.Reset(bs)
	}
}
