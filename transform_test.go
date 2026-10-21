package gio_test

import (
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zalgonoise/gio/v2"
)

func TestMapAndTryMap(t *testing.T) {
	for _, testcase := range [...]struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "MapReaderAndReadItem",
			fn: func(t *testing.T) {
				src := gio.NewReader([]int{1, 2, 3, 4})
				mr := gio.MapReader(src, func(v int) string {
					return strconv.Itoa(v * 10)
				})

				n, err := mr.Read(nil)
				require.NoError(t, err)
				require.Zero(t, n)

				first, err := gio.ReadItem(mr)
				require.NoError(t, err)
				require.Equal(t, "10", first)

				rest, err := gio.ReadAll(mr)
				require.NoError(t, err)
				require.Equal(t, []string{"20", "30", "40"}, rest)

				_, err = gio.ReadItem(mr)
				require.ErrorIs(t, err, gio.EOF)
			},
		},
		{
			name: "TryMapReaderError",
			fn: func(t *testing.T) {
				wantErr := errors.New("parse error")
				src := gio.NewReader([]string{"10", "bad", "30"})
				mr := gio.TryMapReader(src, func(s string) (int, error) {
					if s == "bad" {
						return 0, wantErr
					}
					return strconv.Atoi(s)
				})

				buf := make([]int, 3)
				n, err := mr.Read(buf)
				require.ErrorIs(t, err, wantErr)
				require.Equal(t, 1, n)
				require.Equal(t, 10, buf[0])
			},
		},
		{
			name: "MapWriterAndTryMapWriter",
			fn: func(t *testing.T) {
				dst := gio.NewSliceWriter[int](nil)
				mw := gio.MapWriter(dst, func(s string) int {
					return len(s)
				})

				n, err := mw.Write(nil)
				require.NoError(t, err)
				require.Zero(t, n)

				n, err = mw.Write([]string{"a", "bb", "ccc"})
				require.NoError(t, err)
				require.Equal(t, 3, n)
				require.NoError(t, gio.WriteItem(mw, "dddd"))
				require.Equal(t, []int{1, 2, 3, 4}, dst.Slice())

				// TryMapWriter error mid-batch
				wantErr := errors.New("bad input")
				dst.Reset()
				tmw := gio.TryMapWriter(dst, func(v int) (int, error) {
					if v < 0 {
						return 0, wantErr
					}
					return v * 2, nil
				})
				n, err = tmw.Write([]int{5, -1, 10})
				require.ErrorIs(t, err, wantErr)
				require.Equal(t, 1, n)
				require.Equal(t, []int{10}, dst.Slice())

				require.ErrorIs(t, gio.WriteItem(tmw, -5), wantErr)

				// Short write & writer error on underlying writer
				mwShort := gio.MapWriter(shortWriter[int]{}, func(v int) int { return v })
				_, err = mwShort.Write([]int{1, 2})
				require.ErrorIs(t, err, gio.ErrShortWrite)

				mwErr := gio.MapWriter(errWriter[int]{err: wantErr}, func(v int) int { return v })
				_, err = mwErr.Write([]int{1, 2})
				require.ErrorIs(t, err, wantErr)
			},
		},
	} {
		t.Run(testcase.name, testcase.fn)
	}
}

func TestFilterReaderAndWriter(t *testing.T) {
	for _, testcase := range [...]struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "FilterReaderEvenNumbers",
			fn: func(t *testing.T) {
				// Return 1 item at a time so some reads produce 0 matching items before EOF
				items := []int{1, 3, 4, 5, 6, 7}
				idx := 0
				oneByOne := gio.ReaderFunc[int](func(p []int) (int, error) {
					if idx >= len(items) {
						return 0, gio.EOF
					}
					p[0] = items[idx]
					idx++
					return 1, nil
				})
				fr := gio.FilterReader(oneByOne, func(v int) bool { return v%2 == 0 })

				n, err := fr.Read(nil)
				require.NoError(t, err)
				require.Zero(t, n)

				out, err := gio.ReadAll(fr)
				require.NoError(t, err)
				require.Equal(t, []int{4, 6}, out)

				// ReadItem on FilterReader
				frItem := gio.FilterReader(gio.NewReader([]int{1, 2, 3}), func(v int) bool { return v == 2 })
				v, err := gio.ReadItem(frItem)
				require.NoError(t, err)
				require.Equal(t, 2, v)
				_, err = gio.ReadItem(frItem)
				require.ErrorIs(t, err, gio.EOF)
			},
		},
		{
			name: "FilterReaderNoProgress",
			fn: func(t *testing.T) {
				broken := gio.ReaderFunc[int](func([]int) (int, error) {
					return 0, nil
				})
				fr := gio.FilterReader(broken, func(int) bool { return true })
				buf := make([]int, 4)
				n, err := fr.Read(buf)
				require.ErrorIs(t, err, gio.ErrNoProgress)
				require.Zero(t, n)
			},
		},
		{
			name: "FilterWriter",
			fn: func(t *testing.T) {
				dst := gio.NewSliceWriter[int](nil)
				fw := gio.FilterWriter(dst, func(v int) bool { return v > 0 })

				n, err := fw.Write(nil)
				require.NoError(t, err)
				require.Zero(t, n)

				n, err = fw.Write([]int{-1, 2, -3, 4})
				require.NoError(t, err)
				require.Equal(t, 4, n)

				require.NoError(t, gio.WriteItem(fw, -5))
				require.NoError(t, gio.WriteItem(fw, 6))
				require.Equal(t, []int{2, 4, 6}, dst.Slice())

				// Underlying writer error & short write
				wantErr := errors.New("write err")
				fwErr := gio.FilterWriter(errWriter[int]{err: wantErr}, func(int) bool { return true })
				_, err = fwErr.Write([]int{1})
				require.ErrorIs(t, err, wantErr)

				fwShort := gio.FilterWriter(shortWriter[int]{}, func(int) bool { return true })
				_, err = fwShort.Write([]int{1, 2})
				require.ErrorIs(t, err, gio.ErrShortWrite)
			},
		},
	} {
		t.Run(testcase.name, testcase.fn)
	}
}

func TestBatchAndFlattenReader(t *testing.T) {
	for _, testcase := range [...]struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "BatchAndFlattenRoundTrip",
			fn: func(t *testing.T) {
				input := []int{1, 2, 3, 4, 5, 6, 7}
				br := gio.BatchReader(gio.NewReader(input), 3)

				batches, err := gio.ReadAll(br)
				require.NoError(t, err)
				require.Equal(t, [][]int{{1, 2, 3}, {4, 5, 6}, {7}}, batches)

				// Flatten back with tiny read buffer
				fr := gio.FlattenReader(gio.BatchReader(gio.NewReader(input), 3))
				n, err := fr.Read(nil)
				require.NoError(t, err)
				require.Zero(t, n)

				dst := gio.NewSliceWriter[int](nil)
				_, err = gio.CopyBuffer(dst, fr, make([]int, 2))
				require.NoError(t, err)
				require.Equal(t, input, dst.Slice())
			},
		},
		{
			name: "DefaultBatchSizeAndEmptyFlattenNoProgress",
			fn: func(t *testing.T) {
				br := gio.BatchReader(gio.NewReader([]int{1, 2}), 0)
				batches, err := gio.ReadAll(br)
				require.NoError(t, err)
				require.Equal(t, [][]int{{1, 2}}, batches)

				brokenBatch := gio.ReaderFunc[[]int](func(p [][]int) (int, error) {
					p[0] = nil
					return 1, nil
				})
				fr := gio.FlattenReader(brokenBatch)
				_, err = fr.Read(make([]int, 2))
				require.ErrorIs(t, err, gio.ErrNoProgress)
			},
		},
	} {
		t.Run(testcase.name, testcase.fn)
	}
}

func TestLimitWriter(t *testing.T) {
	for _, testcase := range [...]struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "WithinLimitAndTruncated",
			fn: func(t *testing.T) {
				dst := gio.NewSliceWriter[int](nil)
				lw := gio.LimitWriter(dst, 4)

				n, err := lw.Write(nil)
				require.NoError(t, err)
				require.Zero(t, n)

				n, err = lw.Write([]int{1, 2})
				require.NoError(t, err)
				require.Equal(t, 2, n)

				n, err = lw.Write([]int{3, 4, 5})
				require.ErrorIs(t, err, gio.ErrShortWrite)
				require.Equal(t, 2, n)
				require.Equal(t, []int{1, 2, 3, 4}, dst.Slice())

				// Subsequent writes when N == 0 fail immediately
				n, err = lw.Write([]int{6})
				require.ErrorIs(t, err, gio.ErrShortWrite)
				require.Zero(t, n)
				require.ErrorIs(t, gio.WriteItem[int](lw, 7), gio.ErrShortWrite)

				lwItem := gio.LimitWriter(dst, 1)
				require.NoError(t, gio.WriteItem[int](lwItem, 5))
				require.Equal(t, []int{1, 2, 3, 4, 5}, dst.Slice())
			},
		},
		{
			name: "CustomLimitError",
			fn: func(t *testing.T) {
				customErr := errors.New("quota exceeded")
				dst := gio.NewSliceWriter[int](nil)
				lw := gio.LimitWriter(dst, 1)
				lw.Err = customErr

				n, err := lw.Write([]int{10, 20})
				require.ErrorIs(t, err, customErr)
				require.Equal(t, 1, n)
				require.Equal(t, []int{10}, dst.Slice())
			},
		},
	} {
		t.Run(testcase.name, testcase.fn)
	}
}
