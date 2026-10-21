package gio_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zalgonoise/gio/v2"
)

func TestSliceReader(t *testing.T) {
	for _, testcase := range [...]struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "ReadAndLenSizeReset",
			fn: func(t *testing.T) {
				r := gio.NewReader([]int{10, 20, 30, 40, 50})
				require.Equal(t, 5, r.Len())
				require.Equal(t, int64(5), r.Size())

				buf := make([]int, 3)
				n, err := r.Read(buf)
				require.NoError(t, err)
				require.Equal(t, 3, n)
				require.Equal(t, []int{10, 20, 30}, buf)
				require.Equal(t, 2, r.Len())

				n, err = r.Read(buf)
				require.NoError(t, err)
				require.Equal(t, 2, n)
				require.Equal(t, []int{40, 50}, buf[:2])
				require.Zero(t, r.Len())

				n, err = r.Read(buf)
				require.ErrorIs(t, err, gio.EOF)
				require.Zero(t, n)

				r.Reset([]int{99})
				require.Equal(t, 1, r.Len())
				require.Equal(t, int64(1), r.Size())
			},
		},
		{
			name: "ReadAt",
			fn: func(t *testing.T) {
				r := gio.NewReader([]string{"a", "b", "c", "d"})
				buf := make([]string, 2)

				_, err := r.ReadAt(buf, -1)
				require.ErrorIs(t, err, gio.ErrOffset)

				n, err := r.ReadAt(buf, 4)
				require.ErrorIs(t, err, gio.EOF)
				require.Zero(t, n)

				n, err = r.ReadAt(buf, 1)
				require.NoError(t, err)
				require.Equal(t, 2, n)
				require.Equal(t, []string{"b", "c"}, buf)

				n, err = r.ReadAt(buf, 3)
				require.ErrorIs(t, err, gio.EOF)
				require.Equal(t, 1, n)
				require.Equal(t, "d", buf[0])
			},
		},
		{
			name: "ReadItemAndUnreadItem",
			fn: func(t *testing.T) {
				r := gio.NewReader([]int{7, 8})
				require.ErrorIs(t, r.UnreadItem(), gio.ErrInvalidUnread)

				v, err := r.ReadItem()
				require.NoError(t, err)
				require.Equal(t, 7, v)

				require.NoError(t, r.UnreadItem())
				require.ErrorIs(t, r.UnreadItem(), gio.ErrInvalidUnread)

				v, err = r.ReadItem()
				require.NoError(t, err)
				require.Equal(t, 7, v)

				v, err = r.ReadItem()
				require.NoError(t, err)
				require.Equal(t, 8, v)

				_, err = r.ReadItem()
				require.ErrorIs(t, err, gio.EOF)
			},
		},
		{
			name: "Seek",
			fn: func(t *testing.T) {
				r := gio.NewReader([]int{1, 2, 3, 4})

				pos, err := r.Seek(2, gio.SeekStart)
				require.NoError(t, err)
				require.Equal(t, int64(2), pos)

				pos, err = r.Seek(-1, gio.SeekCurrent)
				require.NoError(t, err)
				require.Equal(t, int64(1), pos)

				pos, err = r.Seek(-1, gio.SeekEnd)
				require.NoError(t, err)
				require.Equal(t, int64(3), pos)

				_, err = r.Seek(-10, gio.SeekStart)
				require.ErrorIs(t, err, gio.ErrOffset)

				_, err = r.Seek(0, 99)
				require.ErrorIs(t, err, gio.ErrWhence)

				// Seek past end makes Len() == 0
				pos, err = r.Seek(10, gio.SeekStart)
				require.NoError(t, err)
				require.Equal(t, int64(10), pos)
				require.Zero(t, r.Len())
			},
		},
		{
			name: "WriteTo",
			fn: func(t *testing.T) {
				r := gio.NewReader([]int{1, 2, 3})
				w := gio.NewSliceWriter[int](nil)

				n, err := r.WriteTo(w)
				require.NoError(t, err)
				require.Equal(t, int64(3), n)
				require.Equal(t, []int{1, 2, 3}, w.Slice())

				// Second call when at EOF returns 0, nil
				n, err = r.WriteTo(w)
				require.NoError(t, err)
				require.Zero(t, n)

				// Short write returns ErrShortWrite
				r.Reset([]int{1, 2, 3})
				n, err = r.WriteTo(shortWriter[int]{})
				require.ErrorIs(t, err, gio.ErrShortWrite)
				require.Equal(t, int64(2), n)

				// Invalid write count panics
				r.Reset([]int{1, 2})
				require.Panics(t, func() {
					_, _ = r.WriteTo(gio.WriterFunc[int](func(p []int) (int, error) {
						return len(p) + 5, nil
					}))
				})
			},
		},
	} {
		t.Run(testcase.name, testcase.fn)
	}
}

func TestSliceWriter(t *testing.T) {
	for _, testcase := range [...]struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "WriteWriteItemLenCapReset",
			fn: func(t *testing.T) {
				w := gio.NewSliceWriter(make([]int, 0, 8))
				require.Zero(t, w.Len())
				require.Equal(t, 8, w.Cap())

				n, err := w.Write([]int{1, 2})
				require.NoError(t, err)
				require.Equal(t, 2, n)
				require.NoError(t, w.WriteItem(3))
				require.Equal(t, []int{1, 2, 3}, w.Slice())
				require.Equal(t, 3, w.Len())

				w.Reset()
				require.Zero(t, w.Len())
				require.Empty(t, w.Slice())
			},
		},
		{
			name: "WriteAtWithinCapAndBeyondCap",
			fn: func(t *testing.T) {
				w := gio.NewSliceWriter(make([]int, 2, 4))
				_, err := w.WriteAt([]int{1}, -1)
				require.ErrorIs(t, err, gio.ErrOffset)

				// Within current len
				n, err := w.WriteAt([]int{10}, 0)
				require.NoError(t, err)
				require.Equal(t, 1, n)
				require.Equal(t, []int{10, 0}, w.Slice())

				// Beyond len but within cap (4)
				n, err = w.WriteAt([]int{20, 30}, 2)
				require.NoError(t, err)
				require.Equal(t, 2, n)
				require.Equal(t, []int{10, 0, 20, 30}, w.Slice())

				// Beyond cap
				n, err = w.WriteAt([]int{50, 60}, 5)
				require.NoError(t, err)
				require.Equal(t, 2, n)
				require.Equal(t, []int{10, 0, 20, 30, 0, 50, 60}, w.Slice())
			},
		},
		{
			name: "ReadFromAndError",
			fn: func(t *testing.T) {
				w := gio.NewSliceWriter[int](nil)
				input := make([]int, 1200)
				for i := range input {
					input[i] = i + 1
				}
				n, err := w.ReadFrom(gio.NewReader(input))
				require.NoError(t, err)
				require.Equal(t, int64(1200), n)
				require.Equal(t, input, w.Slice())

				wantErr := errors.New("readFrom error")
				w.Reset()
				n, err = w.ReadFrom(zeroErrReader[int]{err: wantErr})
				require.ErrorIs(t, err, wantErr)
				require.Equal(t, int64(1), n)

				// Negative read count returns ErrInvalidWrite
				n, err = w.ReadFrom(gio.ReaderFunc[int](func([]int) (int, error) {
					return -1, nil
				}))
				require.ErrorIs(t, err, gio.ErrInvalidWrite)
				require.Zero(t, n)
			},
		},
	} {
		t.Run(testcase.name, testcase.fn)
	}
}
