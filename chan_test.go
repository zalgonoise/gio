package gio_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zalgonoise/gio"
)

func TestChanReaderAndWriter(t *testing.T) {
	for _, testcase := range [...]struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "NilChannelBehavior",
			fn: func(t *testing.T) {
				cr := gio.NewChanReader[int](nil)
				buf := make([]int, 2)
				n, err := cr.Read(buf)
				require.ErrorIs(t, err, gio.EOF)
				require.Zero(t, n)

				_, err = cr.ReadItem()
				require.ErrorIs(t, err, gio.EOF)

				cw := gio.NewChanWriter[int](nil)
				n, err = cw.Write([]int{1})
				require.ErrorIs(t, err, gio.ErrClosedPipe)
				require.Zero(t, n)
				require.ErrorIs(t, cw.WriteItem(1), gio.ErrClosedPipe)
				require.NoError(t, cw.Close())
			},
		},
		{
			name: "BatchDrainAndRoundTrip",
			fn: func(t *testing.T) {
				ch := make(chan int, 10)
				cw := gio.NewChanWriter(ch)
				cr := gio.NewChanReader(ch)

				n, err := cw.Write([]int{1, 2, 3})
				require.NoError(t, err)
				require.Equal(t, 3, n)
				require.NoError(t, cw.WriteItem(4))

				// Empty read returns 0, nil without consuming
				n, err = cr.Read(nil)
				require.NoError(t, err)
				require.Zero(t, n)

				// Buffer of 10 should drain the 4 available items immediately without blocking
				buf := make([]int, 10)
				n, err = cr.Read(buf)
				require.NoError(t, err)
				require.Equal(t, 4, n)
				require.Equal(t, []int{1, 2, 3, 4}, buf[:n])

				// Send 2 more and close writer
				_, err = cw.Write([]int{5, 6})
				require.NoError(t, err)
				require.NoError(t, cw.Close())
				// Second close is a no-op
				require.NoError(t, cw.Close())

				// Writes after close return ErrClosedPipe
				_, err = cw.Write([]int{7})
				require.ErrorIs(t, err, gio.ErrClosedPipe)
				require.ErrorIs(t, cw.WriteItem(7), gio.ErrClosedPipe)

				v, err := cr.ReadItem()
				require.NoError(t, err)
				require.Equal(t, 5, v)

				n, err = cr.Read(buf)
				require.ErrorIs(t, err, gio.EOF)
				require.Equal(t, 1, n)
				require.Equal(t, 6, buf[0])

				n, err = cr.Read(buf)
				require.ErrorIs(t, err, gio.EOF)
				require.Zero(t, n)
			},
		},
		{
			name: "ContextCancellation",
			fn: func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				ch := make(chan int)
				cr := gio.NewChanReaderContext(ctx, ch)
				cw := gio.NewChanWriterContext(ctx, ch)

				// Cancel while blocked waiting
				cancel()

				buf := make([]int, 2)
				n, err := cr.Read(buf)
				require.ErrorIs(t, err, context.Canceled)
				require.Zero(t, n)

				_, err = cr.ReadItem()
				require.ErrorIs(t, err, context.Canceled)

				n, err = cw.Write([]int{1, 2})
				require.ErrorIs(t, err, context.Canceled)
				require.Zero(t, n)

				require.ErrorIs(t, cw.WriteItem(1), context.Canceled)
			},
		},
		{
			name: "ContextActiveAndMidStreamCancel",
			fn: func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				ch := make(chan int, 2)
				cr := gio.NewChanReaderContext(ctx, ch)
				cw := gio.NewChanWriterContext(ctx, ch)

				require.NoError(t, cw.WriteItem(10))
				n, err := cw.Write([]int{20})
				require.NoError(t, err)
				require.Equal(t, 1, n)

				v, err := cr.ReadItem()
				require.NoError(t, err)
				require.Equal(t, 10, v)

				buf := make([]int, 2)
				n, err = cr.Read(buf)
				require.NoError(t, err)
				require.Equal(t, 1, n)
				require.Equal(t, 20, buf[0])

				require.NoError(t, cw.Close())
				_, err = cr.ReadItem()
				require.ErrorIs(t, err, gio.EOF)
			},
		},
	} {
		t.Run(testcase.name, testcase.fn)
	}
}
