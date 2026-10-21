# gio

*A generic, idiomatic I/O library for Go*

---

## Concept

Go's standard [`io`](https://pkg.go.dev/io) package is one of the language's most expressive abstractions: two small interfaces—`io.Reader` and `io.Writer`—power an entire ecosystem of composable streams, buffers, pipes, and transformers. However, the standard `io` package is strictly bound to `[]byte`.

**`gio`** generalizes the `io` design to operate over **any data type `T`** (`[]T` and `T`). By parameterizing `Reader[T]`, `Writer[T]`, and their companion interfaces, `gio` brings the exact semantics, contracts, and composability of standard Go I/O to structured records, domain events, numeric streams, batches, channels, and Go iterators (`iter.Seq` / `iter.Seq2`).

Key design principles:

1. **100% Idiomatic `io` Semantics**: Every interface and helper (`Copy`, `ReadAll`, `SectionReader`, `OffsetWriter`, `MultiReader`, `MultiWriter`, `TeeReader`, `Pipe`, `LimitReader`, `NopCloser`, `Discard`) preserves the behavior and error contracts of the Go 1.27 standard library.
2. **Seamless Standard Library Interop**: Because Go interfaces are satisfied structurally, `gio.Reader[byte]` and `gio.Writer[byte]` are identical in method set to `io.Reader` and `io.Writer`. Any `*os.File`, `*bytes.Buffer`, `*strings.Reader`, or `net.Conn` works directly with `gio[byte]`, including automatic fast-path detection for `io.WriterTo`, `io.ReaderFrom`, `io.StringWriter`, `io.ByteReader`, and `io.ByteWriter`.
3. **Generic-Aware Performance & Memory Safety**:
   - Default internal buffer lengths scale inversely with `unsafe.Sizeof(T)` (~32 KiB target footprint) so streaming large structs never causes multi-megabyte buffer allocations.
   - Temporary and pooled buffers (such as in `Discard[T]`, `BufferedPipe[T]`, and stream transformers) explicitly `clear()` consumed elements so pointer-bearing `T` values are immediately eligible for garbage collection.
4. **Rich Generic Sources, Sinks & Combinators**: Beyond byte streams, `gio` provides zero-dependency primitives for slices (`SliceReader`, `SliceWriter`), closures (`ReaderFunc`, `WriterFunc`), `context.Context` cancellation, Go channels (`ChanReader`, `ChanWriter`), Go `iter` sequences (`Items`, `Chunks`, `FromSeq`, `FromSeq2`), bounded ring-buffer pipes (`BufferedPipe`), cross-type transformations (`MapReader`, `FilterReader`, `BatchReader`, `FlattenReader`), and byte-stream codecs (`NewDecodedReader`, `NewEncodedWriter`).

---

## Installation

```bash
go get github.com/zalgonoise/gio
```

---

## Features

- **Standard `io` Parity (Go 1.27)**:
  - Core interfaces (`Reader[T]`, `Writer[T]`, `Closer`, `Seeker`, `ReaderFrom[T]`, `WriterTo[T]`, `ReaderAt[T]`, `WriterAt[T]`, plus all composite interfaces).
  - Core operations (`Copy`, `CopyBuffer`, `CopyN`, `ReadAtLeast`, `ReadFull`, `ReadAll`, `WriteString`).
  - Stream wrappers (`LimitReader`, `NewSectionReader` with `Outer()`, `NewOffsetWriter`, `TeeReader`, `MultiReader`, `MultiWriter`, `NopCloser`, `Discard[T]()`).
  - Synchronous unbuffered `Pipe[T]()` with single-allocation embedded layout.
- **Single-Item I/O**:
  - `ItemReader[T]`, `ItemScanner[T]`, `ReadItemScanner[T]`, `ItemWriter[T]`.
  - Helper functions `ReadItem[T]`, `WriteItem[T]`, `WriteItems[T]`, and 1-item lookahead wrapper `NewItemScanner[T]`.
- **In-Memory Slice Streams**:
  - `SliceReader[T]` (`NewReader[T]([]T)`) — generic counterpart to `bytes.Reader` implementing `Reader[T]`, `ReaderAt[T]`, `WriterTo[T]`, `Seeker`, and `ItemScanner[T]`.
  - `SliceWriter[T]` (`NewSliceWriter[T]([]T)`) — appendable and random-access (`WriteAt`) slice sink implementing `Writer[T]`, `WriterAt[T]`, `ReaderFrom[T]`, and `ItemWriter[T]`.
- **Functional & Context Wrappers**:
  - `ReaderFunc[T]` and `WriterFunc[T]` for inline closure-based readers and writers.
  - `ContextReader[T]` and `ContextWriter[T]` for `context.Context` cancellation and deadline enforcement.
- **Go Channel Bridges**:
  - `ChanReader[T]` (`NewChanReader`, `NewChanReaderContext`) — blocks for the first item and non-blockingly drains available queued items per `Read`.
  - `ChanWriter[T]` (`NewChanWriter`, `NewChanWriterContext`) — sends written items to `chan<- T` and closes the channel on `Close()`.
- **Go `iter` Integration**:
  - `Items[T](r)` and `Chunks[T](r, size)` returning `iter.Seq2` for `for ... range` loops over any `Reader[T]`.
  - `FromSeq[T](seq)` and `FromSeq2[T](seq)` converting push iterators into pull-based `ReadCloser[T]` streams.
- **Stream Combinators & Transformers**:
  - Cross-type mapping: `MapReader[T, U]`, `TryMapReader[T, U]`, `MapWriter[T, U]`, `TryMapWriter[T, U]`.
  - Predicate filtering: `FilterReader[T]`, `FilterWriter[T]`.
  - Batching & flattening: `BatchReader[T]` (`Reader[T]` $\rightarrow$ `Reader[[]T]`) and `FlattenReader[T]` (`Reader[[]T]` $\rightarrow$ `Reader[T]`).
  - Write limiting: `LimitWriter[T]` (`LimitedWriter[T]`).
- **Buffered Pipe & Byte Codecs**:
  - `BufferedPipe[T](size)` — bounded ring-buffer asynchronous pipe.
  - `NewDecodedReader[T]` and `NewEncodedWriter[T]` — bridge standard `io.Reader` / `io.Writer` byte streams with typed `gio.Reader[T]` / `gio.Writer[T]` pipelines.

---

## API Reference

### Sentinel Errors & Constants

| Identifier | Description |
| :--- | :--- |
| `SeekStart`, `SeekCurrent`, `SeekEnd` | Seek whence constants (aliased from `io`). |
| `EOF` | Graceful end of input (`io.EOF`). |
| `ErrShortWrite` | Write accepted fewer `T` items than requested (`io.ErrShortWrite`). |
| `ErrInvalidWrite` | Write returned an impossible count. |
| `ErrShortBuffer` | Read required a longer buffer than provided (`io.ErrShortBuffer`). |
| `ErrUnexpectedEOF` | `EOF` encountered in the middle of reading a fixed-size block (`io.ErrUnexpectedEOF`). |
| `ErrNoProgress` | Multiple `Read` calls returned no data or error (`io.ErrNoProgress`). |
| `ErrClosedPipe` | Read or write operation on a closed pipe (`io.ErrClosedPipe`). |
| `ErrWhence`, `ErrOffset` | Invalid `Seek` / `ReadAt` / `WriteAt` whence or negative offset. |
| `ErrInvalidUnread` | Invalid use of `UnreadItem()`. |

### Core Interfaces

```go
type Reader[T any] interface { Read(p []T) (n int, err error) }
type Writer[T any] interface { Write(p []T) (n int, err error) }
type Closer = io.Closer
type Seeker = io.Seeker

type ReadWriter[T any] interface { Reader[T]; Writer[T] }
type ReadCloser[T any] interface { Reader[T]; Closer }
type WriteCloser[T any] interface { Writer[T]; Closer }
type ReadWriteCloser[T any] interface { Reader[T]; Writer[T]; Closer }
type ReadSeeker[T any] interface { Reader[T]; Seeker }
type ReadSeekCloser[T any] interface { Reader[T]; Seeker; Closer }
type WriteSeeker[T any] interface { Writer[T]; Seeker }
type ReadWriteSeeker[T any] interface { Reader[T]; Writer[T]; Seeker }

type ReaderFrom[T any] interface { ReadFrom(r Reader[T]) (n int64, err error) }
type WriterTo[T any] interface { WriteTo(w Writer[T]) (n int64, err error) }
type ReaderAt[T any] interface { ReadAt(p []T, off int64) (n int, err error) }
type WriterAt[T any] interface { WriteAt(p []T, off int64) (n int, err error) }

type ItemReader[T any] interface { ReadItem() (T, error) }
type ItemScanner[T any] interface { ItemReader[T]; UnreadItem() error }
type ReadItemScanner[T any] interface { Reader[T]; ItemScanner[T] }
type ItemWriter[T any] interface { WriteItem(item T) error }

// Standard library type aliases
type StringWriter = io.StringWriter
type ByteReader   = io.ByteReader
type ByteScanner  = io.ByteScanner
type ByteWriter   = io.ByteWriter
type RuneReader   = io.RuneReader
type RuneScanner  = io.RuneScanner
```

### Core Functions & Constructors

| Function | Signature | Description |
| :--- | :--- | :--- |
| `Copy` | `Copy[T any](dst Writer[T], src Reader[T]) (int64, error)` | Copies from `src` to `dst` until `EOF` or error, using `WriterTo` or `ReaderFrom` if available. |
| `CopyBuffer` | `CopyBuffer[T any](dst Writer[T], src Reader[T], buf []T) (int64, error)` | Copies using a caller-provided staging buffer `buf`. |
| `CopyN` | `CopyN[T any](dst Writer[T], src Reader[T], n int64) (int64, error)` | Copies exactly `n` `T` items from `src` to `dst`. |
| `ReadAtLeast` | `ReadAtLeast[T any](r Reader[T], buf []T, min int) (int, error)` | Reads at least `min` `T` items into `buf`. |
| `ReadFull` | `ReadFull[T any](r Reader[T], buf []T) (int, error)` | Reads exactly `len(buf)` `T` items into `buf`. |
| `ReadAll` | `ReadAll[T any](r Reader[T]) ([]T, error)` | Reads from `r` until `EOF` using chunked exponential growth. |
| `WriteString` | `WriteString(w Writer[byte], s string) (int, error)` | Writes string `s` to `w` (using `StringWriter` if available). |
| `ReadItem` | `ReadItem[T any](r Reader[T]) (T, error)` | Reads a single `T` item (using `ItemReader` or `ByteReader` if available). |
| `WriteItem` | `WriteItem[T any](w Writer[T], item T) error` | Writes a single `T` item (using `ItemWriter` or `ByteWriter` if available). |
| `WriteItems` | `WriteItems[T any](w Writer[T], items ...T) (int, error)` | Writes variadic `T` items to `w`. |
| `NewItemScanner` | `NewItemScanner[T any](r Reader[T]) ReadItemScanner[T]` | Wraps `r` with 1-item `ReadItem` / `UnreadItem` capability. |
| `LimitReader` | `LimitReader[T any](r Reader[T], n int64) Reader[T]` | Reads from `r` but stops with `EOF` after `n` items. |
| `LimitWriter` | `LimitWriter[T any](w Writer[T], n int64) *LimitedWriter[T]` | Writes up to `n` items to `w`, returning `ErrShortWrite` (or `Err`) afterward. |
| `NewSectionReader` | `NewSectionReader[T any](r ReaderAt[T], off, n int64) *SectionReader[T]` | Reads, seeks, and `ReadAt`s over a section `[off, off+n)` of `r`. |
| `NewOffsetWriter` | `NewOffsetWriter[T any](w WriterAt[T], off int64) *OffsetWriter[T]` | Maps writes and seeks relative to `off` in `w`. |
| `TeeReader` | `TeeReader[T any](r Reader[T], w Writer[T]) Reader[T]` | Writes to `w` everything it reads from `r`. |
| `MultiReader` | `MultiReader[T any](readers ...Reader[T]) Reader[T]` | Logical concatenation of `readers`. |
| `MultiWriter` | `MultiWriter[T any](writers ...Writer[T]) Writer[T]` | Duplicates writes across all `writers`. |
| `NopCloser` | `NopCloser[T any](r Reader[T]) ReadCloser[T]` | Wraps `r` with a no-op `Close()`, preserving `WriterTo`. |
| `Discard` | `Discard[T any]() Writer[T]` | Returns a `Writer[T]` (and `ReaderFrom[T]`) that discards all data. |
| `Pipe` | `Pipe[T any]() (*PipeReader[T], *PipeWriter[T])` | Synchronous unbuffered in-memory pipe. |
| `BufferedPipe` | `BufferedPipe[T any](size int) (*PipeReader[T], *PipeWriter[T])` | Bounded ring-buffer asynchronous in-memory pipe of capacity `size`. |
| `NewReader` | `NewReader[T any](b []T) *SliceReader[T]` | Read-only seekable `SliceReader[T]` over `b`. |
| `NewSliceWriter` | `NewSliceWriter[T any](buf []T) *SliceWriter[T]` | In-memory slice writer/appender backed by `buf`. |
| `ContextReader` | `ContextReader[T any](ctx context.Context, r Reader[T]) Reader[T]` | Checks `ctx.Err()` before each read. |
| `ContextWriter` | `ContextWriter[T any](ctx context.Context, w Writer[T]) Writer[T]` | Checks `ctx.Err()` before each write. |
| `NewChanReader` | `NewChanReader[T any](ch <-chan T) *ChanReader[T]` | Adapts `<-chan T` into a `Reader[T]` and `ItemReader[T]`. |
| `NewChanWriter` | `NewChanWriter[T any](ch chan<- T) *ChanWriter[T]` | Adapts `chan<- T` into a `WriteCloser[T]` and `ItemWriter[T]`. |
| `Items` | `Items[T any](r Reader[T]) iter.Seq2[T, error]` | Returns an iterator yielding individual `T` items from `r`. |
| `Chunks` | `Chunks[T any](r Reader[T], size int) iter.Seq2[[]T, error]` | Returns an iterator yielding slices of up to `size` items from `r`. |
| `FromSeq` | `FromSeq[T any](seq iter.Seq[T]) ReadCloser[T]` | Adapts a push iterator `iter.Seq[T]` into a pull `ReadCloser[T]`. |
| `FromSeq2` | `FromSeq2[T any](seq iter.Seq2[T, error]) ReadCloser[T]` | Adapts an error-yielding `iter.Seq2[T, error]` into a `ReadCloser[T]`. |
| `MapReader` | `MapReader[T, U any](r Reader[T], fn func(T) U) Reader[U]` | Transforms each `T` read from `r` into `U`. |
| `TryMapReader` | `TryMapReader[T, U any](r Reader[T], fn func(T) (U, error)) Reader[U]` | Fallible transformation from `Reader[T]` to `Reader[U]`. |
| `MapWriter` | `MapWriter[T, U any](w Writer[U], fn func(T) U) Writer[T]` | Transforms each written `T` into `U` and writes to `w`. |
| `TryMapWriter` | `TryMapWriter[T, U any](w Writer[U], fn func(T) (U, error)) Writer[T]` | Fallible transformation from `Writer[U]` to `Writer[T]`. |
| `FilterReader` | `FilterReader[T any](r Reader[T], pred func(T) bool) Reader[T]` | Yields only items from `r` matching `pred`. |
| `FilterWriter` | `FilterWriter[T any](w Writer[T], pred func(T) bool) Writer[T]` | Forwards only items matching `pred` to `w`. |
| `BatchReader` | `BatchReader[T any](r Reader[T], batchSize int) Reader[[]T]` | Groups items from `r` into slices of length `batchSize`. |
| `FlattenReader` | `FlattenReader[T any](r Reader[[]T]) Reader[T]` | Flattens a `Reader[[]T]` back into a contiguous `Reader[T]`. |
| `NewDecodedReader` | `NewDecodedReader[T any](r io.Reader, dec func(io.Reader) (T, error)) Reader[T]` | Decodes typed `T` items from a standard `io.Reader`. |
| `NewEncodedWriter` | `NewEncodedWriter[T any](w io.Writer, enc func(io.Writer, T) error) Writer[T]` | Encodes typed `T` items into a standard `io.Writer`. |

---

## Usage Examples

### 1. Streaming, Filtering & Mapping Typed Data

```go
package main

import (
	"fmt"
	"strconv"

	"github.com/zalgonoise/gio"
)

func main() {
	// Create a Reader[int] over a slice
	src := gio.NewReader([]int{1, 2, 3, 4, 5, 6})

	// Filter only even numbers, then map int -> string
	evens := gio.FilterReader(src, func(n int) bool { return n%2 == 0 })
	formatted := gio.MapReader(evens, func(n int) string {
		return "val=" + strconv.Itoa(n*10)
	})

	// Iterate over the resulting Reader[string] using Go range-over-func
	for item, err := range gio.Items(formatted) {
		if err != nil {
			panic(err)
		}
		fmt.Println(item)
	}
	// Output:
	// val=20
	// val=40
	// val=60
}
```

### 2. Bridging Channels and `gio.Copy`

```go
ch := make(chan int, 4)
cw := gio.NewChanWriter(ch)
cr := gio.NewChanReader(ch)

_, _ = gio.WriteItems(cw, 10, 20, 30)
_ = cw.Close()

dst := gio.NewSliceWriter[int](nil)
_, _ = gio.Copy(dst, cr)
// dst.Slice() == []int{10, 20, 30}
```

---

## License

This project is licensed under the MIT License — see the [LICENSE](LICENSE) file for details.
