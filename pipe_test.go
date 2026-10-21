package gio_test

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zalgonoise/gio/v2"
)

func checkWrite(t *testing.T, w gio.Writer[byte], data []byte, c chan int) {
	n, err := w.Write(data)
	require.NoError(t, err)
	require.Equal(t, len(data), n)
	c <- 0
}

func reader(t *testing.T, r gio.Reader[byte], c chan int) {
	buf := make([]byte, 64)
	for {
		n, err := r.Read(buf)
		if err == gio.EOF {
			c <- 0
			break
		}
		require.NoError(t, err)
		c <- n
	}
}

type pipeReturn struct {
	n   int
	err error
}

func writer(w gio.WriteCloser[byte], buf []byte, c chan pipeReturn) {
	n, err := w.Write(buf)
	_ = w.Close()
	c <- pipeReturn{n, err}
}

type closer interface {
	CloseWithError(error) error
	Close() error
}

type pipeTest struct {
	async          bool
	err            error
	closeWithError bool
}

func (p pipeTest) String() string {
	return fmt.Sprintf("async=%v err=%v closeWithError=%v", p.async, p.err, p.closeWithError)
}

var pipeTests = []pipeTest{
	{true, nil, false},
	{true, nil, true},
	{true, gio.ErrShortWrite, true},
	{false, nil, false},
	{false, nil, true},
	{false, gio.ErrShortWrite, true},
}

func delayClose(t *testing.T, cl closer, ch chan int, tt pipeTest) {
	time.Sleep(1 * time.Millisecond)
	var err error
	if tt.closeWithError {
		err = cl.CloseWithError(tt.err)
	} else {
		err = cl.Close()
	}
	require.NoError(t, err)
	ch <- 0
}

func sortBytesInGroups(b []byte, n int) []byte {
	var groups [][]byte
	for len(b) > 0 {
		groups = append(groups, b[:n])
		b = b[n:]
	}
	sort.Slice(groups, func(i, j int) bool { return bytes.Compare(groups[i], groups[j]) < 0 })
	return bytes.Join(groups, nil)
}

func TestPipe(t *testing.T) {
	for _, testcase := range [...]struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "SingleReadWritePair",
			fn: func(t *testing.T) {
				c := make(chan int)
				r, w := gio.Pipe[byte]()
				buf := make([]byte, 64)
				go checkWrite(t, w, []byte("hello, world"), c)
				n, err := r.Read(buf)
				require.NoError(t, err)
				require.Equal(t, 12, n)
				require.Equal(t, "hello, world", string(buf[0:12]))
				<-c
				require.NoError(t, r.Close())
				require.NoError(t, w.Close())
			},
		},
		{
			name: "SequenceOfReadWritePairs",
			fn: func(t *testing.T) {
				c := make(chan int)
				r, w := gio.Pipe[byte]()
				go reader(t, r, c)
				buf := make([]byte, 64)
				for i := 0; i < 5; i++ {
					p := buf[0 : 5+i*10]
					n, err := w.Write(p)
					require.NoError(t, err)
					require.Equal(t, len(p), n)
					nn := <-c
					require.Equal(t, n, nn)
				}
				require.NoError(t, w.Close())
				nn := <-c
				require.Zero(t, nn)
			},
		},
		{
			name: "LargeWriteMultipleReads",
			fn: func(t *testing.T) {
				c := make(chan pipeReturn)
				r, w := gio.Pipe[byte]()
				wdat := make([]byte, 128)
				for i := 0; i < len(wdat); i++ {
					wdat[i] = byte(i)
				}
				go writer(w, wdat, c)
				rdat := make([]byte, 1024)
				tot := 0
				for n := 1; n <= 256; n *= 2 {
					nn, err := r.Read(rdat[tot : tot+n])
					if err != nil {
						require.ErrorIs(t, err, gio.EOF)
					}
					expect := n
					if n == 128 {
						expect = 1
					} else if n == 256 {
						expect = 0
						require.ErrorIs(t, err, gio.EOF)
					}
					require.Equal(t, expect, nn)
					tot += nn
				}
				pr := <-c
				require.NoError(t, pr.err)
				require.Equal(t, 128, pr.n)
				require.Equal(t, 128, tot)
				for i := 0; i < 128; i++ {
					require.Equal(t, byte(i), rdat[i])
				}
			},
		},
		{
			name: "ReadItemAndWriteItem",
			fn: func(t *testing.T) {
				r, w := gio.Pipe[int]()
				go func() {
					require.NoError(t, w.WriteItem(42))
					require.NoError(t, w.Close())
				}()
				v, err := r.ReadItem()
				require.NoError(t, err)
				require.Equal(t, 42, v)
				_, err = r.ReadItem()
				require.ErrorIs(t, err, gio.EOF)
			},
		},
		{
			name: "PipeReadClose",
			fn: func(t *testing.T) {
				for _, tt := range pipeTests {
					c := make(chan int, 1)
					r, w := gio.Pipe[byte]()
					if tt.async {
						go delayClose(t, w, c, tt)
					} else {
						delayClose(t, w, c, tt)
					}
					buf := make([]byte, 64)
					n, err := r.Read(buf)
					<-c
					want := tt.err
					if want == nil {
						want = gio.EOF
					}
					require.ErrorIs(t, err, want)
					require.Zero(t, n)
					require.NoError(t, r.Close())
				}
			},
		},
		{
			name: "PipeReadCloseDuringRead",
			fn: func(t *testing.T) {
				c := make(chan int, 1)
				r, _ := gio.Pipe[byte]()
				go delayClose(t, r, c, pipeTest{})
				n, err := r.Read(make([]byte, 64))
				<-c
				require.ErrorIs(t, err, gio.ErrClosedPipe)
				require.Zero(t, n)
			},
		},
		{
			name: "PipeWriteClose",
			fn: func(t *testing.T) {
				for _, tt := range pipeTests {
					c := make(chan int, 1)
					r, w := gio.Pipe[byte]()
					if tt.async {
						go delayClose(t, r, c, tt)
					} else {
						delayClose(t, r, c, tt)
					}
					n, err := io.WriteString(w, "hello, world")
					<-c
					expect := tt.err
					if expect == nil {
						expect = gio.ErrClosedPipe
					}
					require.ErrorIs(t, err, expect)
					require.Zero(t, n)
					require.NoError(t, w.Close())
				}
			},
		},
		{
			name: "PipeWriteCloseDuringWrite",
			fn: func(t *testing.T) {
				c := make(chan int, 1)
				_, w := gio.Pipe[byte]()
				go delayClose(t, w, c, pipeTest{})
				n, err := w.Write(make([]byte, 64))
				<-c
				require.ErrorIs(t, err, gio.ErrClosedPipe)
				require.Zero(t, n)
			},
		},
		{
			name: "WriteEmptyAndNil",
			fn: func(t *testing.T) {
				r, w := gio.Pipe[byte]()
				go func() {
					_, _ = w.Write([]byte{})
					_ = w.Close()
				}()
				var b [2]byte
				_, _ = gio.ReadFull[byte](r, b[0:2])
				require.NoError(t, r.Close())

				r, w = gio.Pipe[byte]()
				go func() {
					_, _ = w.Write(nil)
					_ = w.Close()
				}()
				_, _ = gio.ReadFull[byte](r, b[0:2])
				require.NoError(t, r.Close())
			},
		},
		{
			name: "WriteAfterWriterClose",
			fn: func(t *testing.T) {
				r, w := gio.Pipe[byte]()
				done := make(chan bool)
				var writeErr error
				go func() {
					_, err := w.Write([]byte("hello"))
					require.NoError(t, err)
					_ = w.Close()
					_, writeErr = w.Write([]byte("world"))
					done <- true
				}()

				buf := make([]byte, 100)
				n, err := gio.ReadFull[byte](r, buf)
				require.ErrorIs(t, err, gio.ErrUnexpectedEOF)
				<-done
				require.Equal(t, "hello", string(buf[0:n]))
				require.ErrorIs(t, writeErr, gio.ErrClosedPipe)
			},
		},
		{
			name: "PipeCloseErrorPreserved",
			fn: func(t *testing.T) {
				type testError1 struct{ error }
				type testError2 struct{ error }

				r, w := gio.Pipe[byte]()
				_ = r.CloseWithError(testError1{})
				_, err := w.Write(nil)
				require.Equal(t, testError1{}, err)
				_ = r.CloseWithError(testError2{})
				_, err = w.Write(nil)
				require.Equal(t, testError1{}, err)

				r, w = gio.Pipe[byte]()
				_ = w.CloseWithError(testError1{})
				_, err = r.Read(nil)
				require.Equal(t, testError1{}, err)
				_ = w.CloseWithError(testError2{})
				_, err = r.Read(nil)
				require.Equal(t, testError1{}, err)
			},
		},
		{
			name: "PipeConcurrent",
			fn: func(t *testing.T) {
				const (
					input    = "0123456789abcdef"
					count    = 8
					readSize = 2
				)

				t.Run("Write", func(t *testing.T) {
					r, w := gio.Pipe[byte]()
					for i := 0; i < count; i++ {
						go func() {
							time.Sleep(time.Millisecond)
							n, err := w.Write([]byte(input))
							require.NoError(t, err)
							require.Equal(t, len(input), n)
						}()
					}

					buf := make([]byte, count*len(input))
					for i := 0; i < len(buf); i += readSize {
						n, err := r.Read(buf[i : i+readSize])
						require.NoError(t, err)
						require.Equal(t, readSize, n)
					}
					require.Equal(t, strings.Repeat(input, count), string(buf))
				})

				t.Run("Read", func(t *testing.T) {
					r, w := gio.Pipe[byte]()
					c := make(chan []byte, count*len(input)/readSize)
					for i := 0; i < cap(c); i++ {
						go func() {
							time.Sleep(time.Millisecond)
							buf := make([]byte, readSize)
							n, err := r.Read(buf)
							require.NoError(t, err)
							require.Equal(t, readSize, n)
							c <- buf
						}()
					}

					for i := 0; i < count; i++ {
						n, err := w.Write([]byte(input))
						require.NoError(t, err)
						require.Equal(t, len(input), n)
					}

					got := make([]byte, 0, count*len(input))
					for i := 0; i < cap(c); i++ {
						got = append(got, (<-c)...)
					}
					got = sortBytesInGroups(got, readSize)
					want := sortBytesInGroups(bytes.Repeat([]byte(input), count), readSize)
					require.Equal(t, string(want), string(got))
				})
			},
		},
	} {
		t.Run(testcase.name, testcase.fn)
	}
}

func TestBufferedPipe(t *testing.T) {
	for _, testcase := range [...]struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "ZeroSizeFallsBackToSyncPipe",
			fn: func(t *testing.T) {
				r, w := gio.BufferedPipe[int](0)
				go func() {
					_, _ = w.Write([]int{1, 2})
					_ = w.Close()
				}()
				out, err := gio.ReadAll(r)
				require.NoError(t, err)
				require.Equal(t, []int{1, 2}, out)
			},
		},
		{
			name: "NonBlockingWriteAndDrainAfterClose",
			fn: func(t *testing.T) {
				r, w := gio.BufferedPipe[int](4)
				// Should not block because buffer capacity is 4
				n, err := w.Write([]int{10, 20, 30})
				require.NoError(t, err)
				require.Equal(t, 3, n)

				// Zero-length write and read
				n, err = w.Write(nil)
				require.NoError(t, err)
				require.Zero(t, n)
				n, err = r.Read(nil)
				require.NoError(t, err)
				require.Zero(t, n)

				// Wrap-around test: read 2 items, write 3 items
				buf := make([]int, 2)
				n, err = r.Read(buf)
				require.NoError(t, err)
				require.Equal(t, 2, n)
				require.Equal(t, []int{10, 20}, buf)

				n, err = w.Write([]int{40, 50, 60})
				require.NoError(t, err)
				require.Equal(t, 3, n)
				require.NoError(t, w.Close())

				// Drain remaining buffered items after writer closed
				out, err := gio.ReadAll(r)
				require.NoError(t, err)
				require.Equal(t, []int{30, 40, 50, 60}, out)
			},
		},
		{
			name: "BlocksWhenFullAndUnblocksOnRead",
			fn: func(t *testing.T) {
				r, w := gio.BufferedPipe[int](2)
				done := make(chan struct{})
				go func() {
					n, err := w.Write([]int{1, 2, 3, 4, 5})
					require.NoError(t, err)
					require.Equal(t, 5, n)
					_ = w.Close()
					close(done)
				}()

				out, err := gio.ReadAll(r)
				require.NoError(t, err)
				require.Equal(t, []int{1, 2, 3, 4, 5}, out)
				<-done
			},
		},
		{
			name: "CloseReadDiscardsAndFailsWrites",
			fn: func(t *testing.T) {
				r, w := gio.BufferedPipe[int](4)
				_, err := w.Write([]int{1, 2})
				require.NoError(t, err)

				customErr := fmt.Errorf("custom read close")
				require.NoError(t, r.CloseWithError(customErr))

				_, err = w.Write([]int{3})
				require.ErrorIs(t, err, customErr)

				buf := make([]int, 2)
				_, err = r.Read(buf)
				require.ErrorIs(t, err, gio.ErrClosedPipe)
			},
		},
		{
			name: "CloseReadWhileWriterBlocked",
			fn: func(t *testing.T) {
				r, w := gio.BufferedPipe[int](2)
				errCh := make(chan error, 1)
				go func() {
					_, err := w.Write([]int{1, 2, 3, 4})
					errCh <- err
				}()
				time.Sleep(2 * time.Millisecond)
				require.NoError(t, r.Close())
				require.ErrorIs(t, <-errCh, gio.ErrClosedPipe)
			},
		},
	} {
		t.Run(testcase.name, testcase.fn)
	}
}
