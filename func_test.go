package gio_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zalgonoise/gio/v2"
)

func TestReaderFuncAndWriterFunc(t *testing.T) {
	rf := gio.ReaderFunc[int](func(p []int) (int, error) {
		p[0] = 42
		return 1, gio.EOF
	})
	var got []int
	wf := gio.WriterFunc[int](func(p []int) (int, error) {
		got = append(got, p...)
		return len(p), nil
	})

	n, err := gio.Copy(wf, rf)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
	require.Equal(t, []int{42}, got)
}

func TestContextReaderAndWriter(t *testing.T) {
	for _, testcase := range [...]struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "NilContextPassthrough",
			fn: func(t *testing.T) {
				r := gio.NewReader([]int{1, 2, 3})
				w := gio.NewSliceWriter[int](nil)
				//nolint:staticcheck // testing nil context handling
				cr := gio.ContextReader(nil, r)
				//nolint:staticcheck // testing nil context handling
				cw := gio.ContextWriter(nil, w)
				require.Same(t, r, cr)
				require.Same(t, w, cw)
			},
		},
		{
			name: "ActiveContextSuccess",
			fn: func(t *testing.T) {
				ctx := context.Background()
				cr := gio.ContextReader(ctx, gio.NewReader([]int{10, 20, 30}))
				w := gio.NewSliceWriter[int](nil)
				cw := gio.ContextWriter(ctx, w)

				v, err := gio.ReadItem(cr)
				require.NoError(t, err)
				require.Equal(t, 10, v)
				require.NoError(t, gio.WriteItem(cw, v))

				n, err := gio.Copy(cw, cr)
				require.NoError(t, err)
				require.Equal(t, int64(2), n)
				require.Equal(t, []int{10, 20, 30}, w.Slice())
			},
		},
		{
			name: "CanceledContextStopsReadAndWrite",
			fn: func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				cr := gio.ContextReader(ctx, gio.NewReader([]int{1, 2}))
				buf := make([]int, 2)
				n, err := cr.Read(buf)
				require.ErrorIs(t, err, context.Canceled)
				require.Zero(t, n)
				_, err = gio.ReadItem(cr)
				require.ErrorIs(t, err, context.Canceled)

				cw := gio.ContextWriter(ctx, gio.NewSliceWriter[int](nil))
				n, err = cw.Write([]int{1, 2})
				require.ErrorIs(t, err, context.Canceled)
				require.Zero(t, n)
				require.ErrorIs(t, gio.WriteItem(cw, 1), context.Canceled)
			},
		},
	} {
		t.Run(testcase.name, testcase.fn)
	}
}
