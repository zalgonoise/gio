package gio_test

import (
	"errors"
	"io"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zalgonoise/gio/v2"
)

func TestItemsAndChunks(t *testing.T) {
	for _, testcase := range [...]struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "ItemsWithItemReaderAndEarlyBreak",
			fn: func(t *testing.T) {
				r := gio.NewReader([]int{10, 20, 30, 40})
				var got []int
				for v, err := range gio.Items(r) {
					require.NoError(t, err)
					got = append(got, v)
					if len(got) == 2 {
						break
					}
				}
				require.Equal(t, []int{10, 20}, got)

				// Read remaining
				for v, err := range gio.Items(r) {
					require.NoError(t, err)
					got = append(got, v)
				}
				require.Equal(t, []int{10, 20, 30, 40}, got)
			},
		},
		{
			name: "ItemsWithPlainReaderAndEarlyBreak",
			fn: func(t *testing.T) {
				r := struct{ gio.Reader[int] }{gio.NewReader([]int{1, 2, 3, 4})}
				var got []int
				for v, err := range gio.Items(r) {
					require.NoError(t, err)
					got = append(got, v)
					if len(got) == 2 {
						break
					}
				}
				require.Equal(t, []int{1, 2}, got)

				rFull := struct{ gio.Reader[int] }{gio.NewReader([]int{5, 6})}
				for v, err := range gio.Items(rFull) {
					require.NoError(t, err)
					got = append(got, v)
				}
				require.Equal(t, []int{1, 2, 5, 6}, got)
			},
		},
		{
			name: "ItemsErrorPropagation",
			fn: func(t *testing.T) {
				wantErr := errors.New("item error")
				// Plain reader error
				rPlain := zeroErrReader[int]{err: wantErr}
				var count int
				var gotErr error
				for _, err := range gio.Items[int](rPlain) {
					if err != nil {
						gotErr = err
						break
					}
					count++
				}
				require.ErrorIs(t, gotErr, wantErr)
				require.Equal(t, 1, count)

				// ItemReader error
				rItem := gio.NewDecodedReader[int](nil, func(io.Reader) (int, error) {
					return 0, wantErr
				})
				gotErr = nil
				for _, err := range gio.Items(rItem) {
					gotErr = err
				}
				require.ErrorIs(t, gotErr, wantErr)
			},
		},
		{
			name: "ChunksCustomAndDefaultSize",
			fn: func(t *testing.T) {
				r := gio.NewReader([]int{1, 2, 3, 4, 5})
				var chunks [][]int
				for chunk, err := range gio.Chunks(r, 2) {
					require.NoError(t, err)
					chunks = append(chunks, slices.Clone(chunk))
				}
				require.Equal(t, [][]int{{1, 2}, {3, 4}, {5}}, chunks)

				// Default size <= 0 and early break
				r2 := gio.NewReader([]int{1, 2, 3})
				var first []int
				for chunk, err := range gio.Chunks(r2, 0) {
					require.NoError(t, err)
					first = slices.Clone(chunk)
					break
				}
				require.Equal(t, []int{1, 2, 3}, first)

				// Error propagation in Chunks
				wantErr := errors.New("chunk err")
				var gotErr error
				for _, err := range gio.Chunks[int](zeroErrReader[int]{err: wantErr}, 2) {
					if err != nil {
						gotErr = err
					}
				}
				require.ErrorIs(t, gotErr, wantErr)
			},
		},
	} {
		t.Run(testcase.name, testcase.fn)
	}
}

func TestFromSeqAndFromSeq2(t *testing.T) {
	for _, testcase := range [...]struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "FromSeqReadAndReadItem",
			fn: func(t *testing.T) {
				seq := slices.Values([]string{"a", "b", "c", "d"})
				r := gio.FromSeq(seq)
				defer func() { _ = r.Close() }()

				v, err := gio.ReadItem[string](r)
				require.NoError(t, err)
				require.Equal(t, "a", v)

				rest, err := gio.ReadAll[string](r)
				require.NoError(t, err)
				require.Equal(t, []string{"b", "c", "d"}, rest)

				// Subsequent Read and ReadItem after exhaustion return EOF
				_, err = gio.ReadItem[string](r)
				require.ErrorIs(t, err, gio.EOF)
				n, err := r.Read(make([]string, 2))
				require.ErrorIs(t, err, gio.EOF)
				require.Zero(t, n)
			},
		},
		{
			name: "FromSeqEarlyClose",
			fn: func(t *testing.T) {
				seq := slices.Values([]int{1, 2, 3})
				r := gio.FromSeq(seq)
				require.NoError(t, r.Close())
				_, err := gio.ReadItem[int](r)
				require.ErrorIs(t, err, gio.EOF)
			},
		},
		{
			name: "FromSeq2SuccessAndErrors",
			fn: func(t *testing.T) {
				seq := gio.Items(gio.NewReader([]int{10, 20, 30}))
				r := gio.FromSeq2(seq)

				v, err := gio.ReadItem[int](r)
				require.NoError(t, err)
				require.Equal(t, 10, v)

				rest, err := gio.ReadAll[int](r)
				require.NoError(t, err)
				require.Equal(t, []int{20, 30}, rest)
				require.NoError(t, r.Close())

				// Sequence yielding explicit EOF and non-EOF error
				wantErr := errors.New("seq2 fail")
				errSeq := func(yield func(int, error) bool) {
					if !yield(1, nil) {
						return
					}
					if !yield(0, wantErr) {
						return
					}
				}
				rErr := gio.FromSeq2(errSeq)
				buf := make([]int, 4)
				n, err := rErr.Read(buf)
				require.ErrorIs(t, err, wantErr)
				require.Equal(t, 1, n)
				require.Equal(t, 1, buf[0])
				require.NoError(t, rErr.Close())

				// ReadItem on closed or EOF seq2
				_, err = gio.ReadItem[int](rErr)
				require.ErrorIs(t, err, gio.EOF)

				eofSeq := func(yield func(int, error) bool) {
					yield(0, gio.EOF)
				}
				rEOF := gio.FromSeq2(eofSeq)
				_, err = gio.ReadItem[int](rEOF)
				require.ErrorIs(t, err, gio.EOF)
			},
		},
	} {
		t.Run(testcase.name, testcase.fn)
	}
}
