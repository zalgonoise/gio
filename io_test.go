package gio_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	. "github.com/zalgonoise/gio"
)

// A version of bytes.Buffer without ReadFrom and WriteTo
type Buffer struct {
	bytes.Buffer
	ReaderFrom[byte] // conflicts with and hides bytes.Buffer's ReaderFrom.
	WriterTo[byte]   // conflicts with and hides bytes.Buffer's WriterTo.
}

// Simple tests, primarily to verify the ReadFrom and WriteTo callouts inside Copy, CopyBuffer and CopyN.

func TestCopy(t *testing.T) {
	rb := new(Buffer)
	wb := new(Buffer)
	rb.WriteString("hello, world.")
	Copy[byte](wb, rb)
	if wb.String() != "hello, world." {
		t.Errorf("Copy did not work properly")
	}
}

func TestCopyNegative(t *testing.T) {
	rb := new(Buffer)
	wb := new(Buffer)
	rb.WriteString("hello")
	Copy[byte](wb, &LimitedReader[byte]{R: rb, N: -1})
	if wb.String() != "" {
		t.Errorf("Copy on LimitedReader with N<0 copied data")
	}

	CopyN[byte](wb, rb, -1)
	if wb.String() != "" {
		t.Errorf("CopyN with N<0 copied data")
	}
}

func TestCopyBuffer(t *testing.T) {
	rb := new(Buffer)
	wb := new(Buffer)
	rb.WriteString("hello, world.")
	CopyBuffer[byte](wb, rb, make([]byte, 1)) // Tiny buffer to keep it honest.
	if wb.String() != "hello, world." {
		t.Errorf("CopyBuffer did not work properly")
	}
}

func TestCopyBufferNil(t *testing.T) {
	rb := new(Buffer)
	wb := new(Buffer)
	rb.WriteString("hello, world.")
	CopyBuffer[byte](wb, rb, nil) // Should allocate a buffer.
	if wb.String() != "hello, world." {
		t.Errorf("CopyBuffer did not work properly")
	}
}

func TestCopyReadFrom(t *testing.T) {
	rb := new(Buffer)
	wb := new(bytes.Buffer) // implements ReadFrom.
	rb.WriteString("hello, world.")
	Copy[byte](wb, rb)
	if wb.String() != "hello, world." {
		t.Errorf("Copy did not work properly")
	}
}

func TestCopyWriteTo(t *testing.T) {
	rb := new(bytes.Buffer) // implements WriteTo.
	wb := new(Buffer)
	rb.WriteString("hello, world.")
	Copy[byte](wb, rb)
	if wb.String() != "hello, world." {
		t.Errorf("Copy did not work properly")
	}
}

// Version of bytes.Buffer that checks whether WriteTo was called or not
type writeToChecker struct {
	bytes.Buffer
	writeToCalled bool
}

func (wt *writeToChecker) WriteTo(w Writer[byte]) (int64, error) {
	wt.writeToCalled = true
	return wt.Buffer.WriteTo(w)
}

// It's preferable to choose WriterTo over ReaderFrom, since a WriterTo can issue one large write,
// while the ReaderFrom must read until EOF, potentially allocating when running out of buffer.
// Make sure that we choose WriterTo when both are implemented.
func TestCopyPriority(t *testing.T) {
	rb := new(writeToChecker)
	wb := new(bytes.Buffer)
	rb.WriteString("hello, world.")
	Copy[byte](wb, rb)
	if wb.String() != "hello, world." {
		t.Errorf("Copy did not work properly")
	} else if !rb.writeToCalled {
		t.Errorf("WriteTo was not prioritized over ReadFrom")
	}
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

// In case a Read results in an error with non-zero bytes read, and
// the subsequent Write also results in an error, the error from Write
// is returned, as it is the one that prevented progressing further.
func TestCopyReadErrWriteErr(t *testing.T) {
	er, ew := errors.New("readError"), errors.New("writeError")
	r, w := zeroErrReader[byte]{err: er}, errWriter[byte]{err: ew}
	n, err := Copy[byte](w, r)
	if n != 0 || err != ew {
		t.Errorf("Copy(zeroErrReader, errWriter) = %d, %v; want 0, writeError", n, err)
	}
}

func TestCopyN(t *testing.T) {
	rb := new(Buffer)
	wb := new(Buffer)
	rb.WriteString("hello, world.")
	CopyN[byte](wb, rb, 5)
	if wb.String() != "hello" {
		t.Errorf("CopyN did not work properly")
	}
}

func TestCopyNReadFrom(t *testing.T) {
	rb := new(Buffer)
	wb := new(bytes.Buffer) // implements ReadFrom.
	rb.WriteString("hello")
	CopyN[byte](wb, rb, 5)
	if wb.String() != "hello" {
		t.Errorf("CopyN did not work properly")
	}
}

func TestCopyNWriteTo(t *testing.T) {
	rb := new(bytes.Buffer) // implements WriteTo.
	wb := new(Buffer)
	rb.WriteString("hello, world.")
	CopyN[byte](wb, rb, 5)
	if wb.String() != "hello" {
		t.Errorf("CopyN did not work properly")
	}
}

func BenchmarkCopyNSmall(b *testing.B) {
	bs := bytes.Repeat([]byte{0}, 512+1)
	rd := bytes.NewReader(bs)
	buf := new(Buffer)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		CopyN[byte](buf, rd, 512)
		rd.Reset(bs)
	}
}

func BenchmarkCopyNLarge(b *testing.B) {
	bs := bytes.Repeat([]byte{0}, (32*1024)+1)
	rd := bytes.NewReader(bs)
	buf := new(Buffer)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		CopyN[byte](buf, rd, 32*1024)
		rd.Reset(bs)
	}
}

type noReadFrom[T any] struct {
	w Writer[T]
}

func (w *noReadFrom[T]) Write(p []T) (n int, err error) {
	return w.w.Write(p)
}

type wantedAndErrReader[T any] struct{}

func (wantedAndErrReader[T]) Read(p []T) (int, error) {
	return len(p), errors.New("wantedAndErrReader error")
}

func TestCopyNEOF(t *testing.T) {
	// Test that EOF behavior is the same regardless of whether
	// argument to CopyN has ReadFrom.

	b := new(bytes.Buffer)

	n, err := CopyN[byte](&noReadFrom[byte]{b}, strings.NewReader("foo"), 3)
	if n != 3 || err != nil {
		t.Errorf("CopyN(noReadFrom, foo, 3) = %d, %v; want 3, nil", n, err)
	}

	n, err = CopyN[byte](&noReadFrom[byte]{b}, strings.NewReader("foo"), 4)
	if n != 3 || err != io.EOF {
		t.Errorf("CopyN(noReadFrom, foo, 4) = %d, %v; want 3, EOF", n, err)
	}

	n, err = CopyN[byte](b, strings.NewReader("foo"), 3) // b has read from
	if n != 3 || err != nil {
		t.Errorf("CopyN(bytes.Buffer, foo, 3) = %d, %v; want 3, nil", n, err)
	}

	n, err = CopyN[byte](b, strings.NewReader("foo"), 4) // b has read from
	if n != 3 || err != io.EOF {
		t.Errorf("CopyN(bytes.Buffer, foo, 4) = %d, %v; want 3, EOF", n, err)
	}

	n, err = CopyN[byte](b, wantedAndErrReader[byte]{}, 5)
	if n != 5 || err != nil {
		t.Errorf("CopyN(bytes.Buffer, wantedAndErrReader, 5) = %d, %v; want 5, nil", n, err)
	}

	n, err = CopyN[byte](&noReadFrom[byte]{b}, wantedAndErrReader[byte]{}, 5)
	if n != 5 || err != nil {
		t.Errorf("CopyN(noReadFrom, wantedAndErrReader, 5) = %d, %v; want 5, nil", n, err)
	}
}

func TestReadAtLeast(t *testing.T) {
	var rb bytes.Buffer
	testReadAtLeast(t, &rb)
}

// A version of bytes.Buffer that returns n > 0, err on Read
// when the input is exhausted.
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

func TestReadAtLeastWithDataAndEOF(t *testing.T) {
	var rb dataAndErrorBuffer
	rb.err = io.EOF
	testReadAtLeast(t, &rb)
}

func TestReadAtLeastWithDataAndError(t *testing.T) {
	var rb dataAndErrorBuffer
	rb.err = fmt.Errorf("fake error")
	testReadAtLeast(t, &rb)
}

func testReadAtLeast(t *testing.T, rb ReadWriter[byte]) {
	rb.Write([]byte("0123"))
	buf := make([]byte, 2)
	n, err := ReadAtLeast[byte](rb, buf, 2)
	if err != nil {
		t.Error(err)
	}
	if n != 2 {
		t.Errorf("expected to have read 2 bytes, got %v", n)
	}
	n, err = ReadAtLeast[byte](rb, buf, 4)
	if err != ErrShortBuffer {
		t.Errorf("expected ErrShortBuffer got %v", err)
	}
	if n != 0 {
		t.Errorf("expected to have read 0 bytes, got %v", n)
	}
	n, err = ReadAtLeast[byte](rb, buf, 1)
	if err != nil {
		t.Error(err)
	}
	if n != 2 {
		t.Errorf("expected to have read 2 bytes, got %v", n)
	}
	n, err = ReadAtLeast[byte](rb, buf, 2)
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
	if n != 0 {
		t.Errorf("expected to have read 0 bytes, got %v", n)
	}
	rb.Write([]byte("4"))
	n, err = ReadAtLeast[byte](rb, buf, 2)
	want := ErrUnexpectedEOF
	if rb, ok := rb.(*dataAndErrorBuffer); ok && rb.err != io.EOF {
		want = rb.err
	}
	if err != want {
		t.Errorf("expected %v, got %v", want, err)
	}
	if n != 1 {
		t.Errorf("expected to have read 1 bytes, got %v", n)
	}
}

func TestTeeReader(t *testing.T) {
	src := []byte("hello, world")
	dst := make([]byte, len(src))
	rb := bytes.NewBuffer(src)
	wb := new(bytes.Buffer)
	r := TeeReader[byte](rb, wb)
	if n, err := ReadFull(r, dst); err != nil || n != len(src) {
		t.Fatalf("ReadFull(r, dst) = %d, %v; want %d, nil", n, err, len(src))
	}
	if !bytes.Equal(dst, src) {
		t.Errorf("bytes read = %q want %q", dst, src)
	}
	if !bytes.Equal(wb.Bytes(), src) {
		t.Errorf("bytes written = %q want %q", wb.Bytes(), src)
	}
	if n, err := r.Read(dst); n != 0 || err != io.EOF {
		t.Errorf("r.Read at EOF = %d, %v want 0, EOF", n, err)
	}
	rb = bytes.NewBuffer(src)
	pr, pw := Pipe[byte]()
	pr.Close()
	r = TeeReader[byte](rb, pw)
	if n, err := ReadFull(r, dst); n != 0 || err != ErrClosedPipe {
		t.Errorf("closed tee: ReadFull(r, dst) = %d, %v; want 0, EPIPE", n, err)
	}
}

func TestSectionReader_ReadAt(t *testing.T) {
	dat := "a long sample data, 1234567890"
	tests := []struct {
		data   string
		off    int
		n      int
		bufLen int
		at     int
		exp    string
		err    error
	}{
		{data: "", off: 0, n: 10, bufLen: 2, at: 0, exp: "", err: io.EOF},
		{data: dat, off: 0, n: len(dat), bufLen: 0, at: 0, exp: "", err: nil},
		{data: dat, off: len(dat), n: 1, bufLen: 1, at: 0, exp: "", err: io.EOF},
		{data: dat, off: 0, n: len(dat) + 2, bufLen: len(dat), at: 0, exp: dat, err: nil},
		{data: dat, off: 0, n: len(dat), bufLen: len(dat) / 2, at: 0, exp: dat[:len(dat)/2], err: nil},
		{data: dat, off: 0, n: len(dat), bufLen: len(dat), at: 0, exp: dat, err: nil},
		{data: dat, off: 0, n: len(dat), bufLen: len(dat) / 2, at: 2, exp: dat[2 : 2+len(dat)/2], err: nil},
		{data: dat, off: 3, n: len(dat), bufLen: len(dat) / 2, at: 2, exp: dat[5 : 5+len(dat)/2], err: nil},
		{data: dat, off: 3, n: len(dat) / 2, bufLen: len(dat)/2 - 2, at: 2, exp: dat[5 : 5+len(dat)/2-2], err: nil},
		{data: dat, off: 3, n: len(dat) / 2, bufLen: len(dat)/2 + 2, at: 2, exp: dat[5 : 5+len(dat)/2-2], err: io.EOF},
		{data: dat, off: 0, n: 0, bufLen: 0, at: -1, exp: "", err: io.EOF},
		{data: dat, off: 0, n: 0, bufLen: 0, at: 1, exp: "", err: io.EOF},
	}
	for i, tt := range tests {
		r := strings.NewReader(tt.data)
		s := NewSectionReader[byte](r, int64(tt.off), int64(tt.n))
		buf := make([]byte, tt.bufLen)
		if n, err := s.ReadAt(buf, int64(tt.at)); n != len(tt.exp) || string(buf[:n]) != tt.exp || err != tt.err {
			t.Fatalf("%d: ReadAt(%d) = %q, %v; expected %q, %v", i, tt.at, buf[:n], err, tt.exp, tt.err)
		}
	}
}

func TestSectionReader_Seek(t *testing.T) {
	// Verifies that NewSectionReader's Seeker behaves like bytes.NewReader (which is like strings.NewReader)
	br := bytes.NewReader([]byte("foo"))
	sr := NewSectionReader[byte](br, 0, int64(len("foo")))

	for _, whence := range []int{SeekStart, SeekCurrent, SeekEnd} {
		for offset := int64(-3); offset <= 4; offset++ {
			brOff, brErr := br.Seek(offset, whence)
			srOff, srErr := sr.Seek(offset, whence)
			if (brErr != nil) != (srErr != nil) || brOff != srOff {
				t.Errorf("For whence %d, offset %d: bytes.Reader.Seek = (%v, %v) != SectionReader.Seek = (%v, %v)",
					whence, offset, brOff, brErr, srErr, srOff)
			}
		}
	}

	// And verify we can just seek past the end and get an EOF
	got, err := sr.Seek(100, SeekStart)
	if err != nil || got != 100 {
		t.Errorf("Seek = %v, %v; want 100, nil", got, err)
	}

	n, err := sr.Read(make([]byte, 10))
	if n != 0 || err != io.EOF {
		t.Errorf("Read = %v, %v; want 0, EOF", n, err)
	}
}

func TestSectionReader_Size(t *testing.T) {
	tests := []struct {
		data string
		want int64
	}{
		{"a long sample data, 1234567890", 30},
		{"", 0},
	}

	for _, tt := range tests {
		r := strings.NewReader(tt.data)
		sr := NewSectionReader[byte](r, 0, int64(len(tt.data)))
		if got := sr.Size(); got != tt.want {
			t.Errorf("Size = %v; want %v", got, tt.want)
		}
	}
}

func TestSectionReader_Max(t *testing.T) {
	r := strings.NewReader("abcdef")
	const maxint64 = 1<<63 - 1
	sr := NewSectionReader[byte](r, 3, maxint64)
	n, err := sr.Read(make([]byte, 3))
	if n != 3 || err != nil {
		t.Errorf("Read = %v %v, want 3, nil", n, err)
	}
	n, err = sr.Read(make([]byte, 3))
	if n != 0 || err != io.EOF {
		t.Errorf("Read = %v, %v, want 0, EOF", n, err)
	}
}

// largeWriter returns an invalid count that is larger than the number
// of bytes provided (issue 39978).
type largeWriter struct {
	err error
}

func (w largeWriter) Write(p []byte) (int, error) {
	return len(p) + 1, w.err
}

func TestCopyLargeWriter(t *testing.T) {
	want := ErrInvalidWrite
	rb := new(Buffer)
	wb := largeWriter{}
	rb.WriteString("hello, world.")
	if _, err := Copy[byte](wb, rb); err != want {
		t.Errorf("Copy error: got %v, want %v", err, want)
	}

	want = errors.New("largeWriterError")
	rb = new(Buffer)
	wb = largeWriter{err: want}
	rb.WriteString("hello, world.")
	if _, err := Copy[byte](wb, rb); err != want {
		t.Errorf("Copy error: got %v, want %v", err, want)
	}
}

func TestNopCloserWriterToForwarding(t *testing.T) {
	for _, tc := range [...]struct {
		Name string
		r    Reader[byte]
	}{
		{"not a WriterTo", Reader[byte](nil)},
		{"a WriterTo", struct {
			Reader[byte]
			WriterTo[byte]
		}{}},
	} {
		nc := NopCloser(tc.r)

		_, expected := tc.r.(WriterTo[byte])
		_, got := nc.(WriterTo[byte])
		if expected != got {
			t.Errorf("NopCloser incorrectly forwards WriterTo for %s, got %t want %t", tc.Name, got, expected)
		}
	}
}
