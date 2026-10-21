package gio_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zalgonoise/gio/v2"
)

func TestReadItemAndWriteItem(t *testing.T) {
	for _, testcase := range [...]struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "ItemReaderAndItemWriter",
			fn: func(t *testing.T) {
				r := gio.NewReader([]string{"alpha", "beta"})
				w := gio.NewSliceWriter[string](nil)

				item, err := gio.ReadItem(r)
				require.NoError(t, err)
				require.Equal(t, "alpha", item)
				require.NoError(t, gio.WriteItem(w, item))

				item, err = gio.ReadItem(r)
				require.NoError(t, err)
				require.Equal(t, "beta", item)
				require.NoError(t, gio.WriteItem(w, item))

				_, err = gio.ReadItem(r)
				require.ErrorIs(t, err, gio.EOF)
				require.Equal(t, []string{"alpha", "beta"}, w.Slice())
			},
		},
		{
			name: "StdlibByteReaderAndByteWriter",
			fn: func(t *testing.T) {
				r := strings.NewReader("xy")
				var w bytes.Buffer

				b1, err := gio.ReadItem[byte](r)
				require.NoError(t, err)
				require.Equal(t, byte('x'), b1)
				require.NoError(t, gio.WriteItem[byte](&w, b1))

				b2, err := gio.ReadItem[byte](r)
				require.NoError(t, err)
				require.Equal(t, byte('y'), b2)
				require.NoError(t, gio.WriteItem[byte](&w, b2))

				require.Equal(t, "xy", w.String())
			},
		},
		{
			name: "FallbackReaderAndWriter",
			fn: func(t *testing.T) {
				r := struct{ gio.Reader[int] }{gio.NewReader([]int{42})}
				sw := gio.NewSliceWriter[int](nil)
				w := struct{ gio.Writer[int] }{sw}

				v, err := gio.ReadItem(r)
				require.NoError(t, err)
				require.Equal(t, 42, v)

				require.NoError(t, gio.WriteItem(w, v))
				require.Equal(t, []int{42}, sw.Slice())
			},
		},
		{
			name: "WriteItemFallbackErrors",
			fn: func(t *testing.T) {
				wantErr := errors.New("write fail")
				err := gio.WriteItem[int](errWriter[int]{err: wantErr}, 1)
				require.ErrorIs(t, err, wantErr)

				err = gio.WriteItem[int](shortWriter[int]{}, 1)
				require.ErrorIs(t, err, gio.ErrShortWrite)
			},
		},
		{
			name: "WriteItemsVariadic",
			fn: func(t *testing.T) {
				w := gio.NewSliceWriter[int](nil)
				n, err := gio.WriteItems(w)
				require.NoError(t, err)
				require.Zero(t, n)

				n, err = gio.WriteItems(w, 10, 20, 30)
				require.NoError(t, err)
				require.Equal(t, 3, n)
				require.Equal(t, []int{10, 20, 30}, w.Slice())
			},
		},
	} {
		t.Run(testcase.name, testcase.fn)
	}
}

func TestItemScanner(t *testing.T) {
	for _, testcase := range [...]struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "ReturnsExistingReadItemScanner",
			fn: func(t *testing.T) {
				sr := gio.NewReader([]int{1, 2, 3})
				scanner := gio.NewItemScanner(sr)
				require.Same(t, sr, scanner)
			},
		},
		{
			name: "WrapsPlainReader",
			fn: func(t *testing.T) {
				plain := struct{ gio.Reader[int] }{gio.NewReader([]int{10, 20, 30})}
				scanner := gio.NewItemScanner(plain)

				// Unread before any ReadItem fails
				require.ErrorIs(t, scanner.UnreadItem(), gio.ErrInvalidUnread)

				v, err := scanner.ReadItem()
				require.NoError(t, err)
				require.Equal(t, 10, v)

				require.NoError(t, scanner.UnreadItem())
				// Double UnreadItem fails
				require.ErrorIs(t, scanner.UnreadItem(), gio.ErrInvalidUnread)

				// Next ReadItem returns unread item
				v, err = scanner.ReadItem()
				require.NoError(t, err)
				require.Equal(t, 10, v)

				// Unread again and consume via Read([]T)
				require.NoError(t, scanner.UnreadItem())
				buf := make([]int, 2)
				n, err := scanner.Read(buf[:0])
				require.NoError(t, err)
				require.Zero(t, n)

				n, err = scanner.Read(buf)
				require.NoError(t, err)
				require.Equal(t, 1, n)
				require.Equal(t, 10, buf[0])

				// Read remaining items via Read
				n, err = scanner.Read(buf)
				require.NoError(t, err)
				require.Equal(t, 2, n)
				require.Equal(t, []int{20, 30}, buf)

				// Unread last item read by Read
				require.NoError(t, scanner.UnreadItem())
				v, err = scanner.ReadItem()
				require.NoError(t, err)
				require.Equal(t, 30, v)

				_, err = scanner.ReadItem()
				require.ErrorIs(t, err, gio.EOF)
			},
		},
	} {
		t.Run(testcase.name, testcase.fn)
	}
}
