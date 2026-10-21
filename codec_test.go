package gio_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zalgonoise/gio/v2"
)

func TestDecodedReaderAndEncodedWriter(t *testing.T) {
	encodeInt64 := func(w io.Writer, v int64) error {
		return binary.Write(w, binary.LittleEndian, v)
	}
	decodeInt64 := func(r io.Reader) (int64, error) {
		var v int64
		err := binary.Read(r, binary.LittleEndian, &v)
		return v, err
	}

	for _, testcase := range [...]struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "RoundTripInt64Stream",
			fn: func(t *testing.T) {
				var buf bytes.Buffer
				ew := gio.NewEncodedWriter(&buf, encodeInt64)
				dr := gio.NewDecodedReader(&buf, decodeInt64)

				n, err := ew.Write([]int64{100, 200, 300})
				require.NoError(t, err)
				require.Equal(t, 3, n)
				require.NoError(t, gio.WriteItem(ew, 400))

				first, err := gio.ReadItem(dr)
				require.NoError(t, err)
				require.Equal(t, int64(100), first)

				rest, err := gio.ReadAll(dr)
				require.NoError(t, err)
				require.Equal(t, []int64{200, 300, 400}, rest)
			},
		},
		{
			name: "ErrorPropagation",
			fn: func(t *testing.T) {
				wantErr := errors.New("codec failure")
				ew := gio.NewEncodedWriter(&bytes.Buffer{}, func(io.Writer, int) error {
					return wantErr
				})
				n, err := ew.Write([]int{1, 2})
				require.ErrorIs(t, err, wantErr)
				require.Zero(t, n)

				dr := gio.NewDecodedReader(&bytes.Buffer{}, func(io.Reader) (int, error) {
					return 0, wantErr
				})
				buf := make([]int, 2)
				n, err = dr.Read(buf)
				require.ErrorIs(t, err, wantErr)
				require.Zero(t, n)
			},
		},
	} {
		t.Run(testcase.name, testcase.fn)
	}
}
