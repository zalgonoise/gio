package gio

import (
	"io"
	"sync"
)

// onceError is an object that will only store an error once.
type onceError struct {
	sync.Mutex // guards following
	err        error
}

func (a *onceError) Store(err error) {
	a.Lock()
	defer a.Unlock()
	if a.err != nil {
		return
	}
	a.err = err
}

func (a *onceError) Load() error {
	a.Lock()
	defer a.Unlock()
	return a.err
}

// ErrClosedPipe is the error used for read or write operations on a closed pipe.
var ErrClosedPipe = io.ErrClosedPipe

// A pipe is the shared pipe structure underlying PipeReader and PipeWriter.
type pipe[T any] struct {
	wrMu sync.Mutex // Serializes Write operations
	wrCh chan []T
	rdCh chan int

	once sync.Once // Protects closing done
	done chan struct{}
	rerr onceError
	werr onceError

	bp *bufferedPipe[T]
}

func (p *pipe[T]) read(b []T) (n int, err error) {
	if p.bp != nil {
		return p.bp.read(b)
	}
	select {
	case <-p.done:
		return 0, p.readCloseError()
	default:
	}

	select {
	case bw := <-p.wrCh:
		nr := copy(b, bw)
		p.rdCh <- nr
		return nr, nil
	case <-p.done:
		return 0, p.readCloseError()
	}
}

func (p *pipe[T]) closeRead(err error) error {
	if p.bp != nil {
		return p.bp.closeRead(err)
	}
	if err == nil {
		err = ErrClosedPipe
	}
	p.rerr.Store(err)
	p.once.Do(func() { close(p.done) })
	return nil
}

func (p *pipe[T]) write(b []T) (n int, err error) {
	if p.bp != nil {
		return p.bp.write(b)
	}
	select {
	case <-p.done:
		return 0, p.writeCloseError()
	default:
		p.wrMu.Lock()
		defer p.wrMu.Unlock()
	}

	for once := true; once || len(b) > 0; once = false {
		select {
		case p.wrCh <- b:
			nw := <-p.rdCh
			b = b[nw:]
			n += nw
		case <-p.done:
			return n, p.writeCloseError()
		}
	}
	return n, nil
}

func (p *pipe[T]) closeWrite(err error) error {
	if p.bp != nil {
		return p.bp.closeWrite(err)
	}
	if err == nil {
		err = EOF
	}
	p.werr.Store(err)
	p.once.Do(func() { close(p.done) })
	return nil
}

// readCloseError is considered internal to the pipe type.
func (p *pipe[T]) readCloseError() error {
	rerr := p.rerr.Load()
	if werr := p.werr.Load(); rerr == nil && werr != nil {
		return werr
	}
	return ErrClosedPipe
}

// writeCloseError is considered internal to the pipe type.
func (p *pipe[T]) writeCloseError() error {
	werr := p.werr.Load()
	if rerr := p.rerr.Load(); werr == nil && rerr != nil {
		return rerr
	}
	return ErrClosedPipe
}

// A PipeReader is the read half of a pipe.
type PipeReader[T any] struct {
	pipe[T]
}

// Read implements the standard Read interface:
// it reads data from the pipe, blocking until a writer
// arrives or the write end is closed.
// If the write end is closed with an error, that error is
// returned as err; otherwise err is EOF.
func (r *PipeReader[T]) Read(data []T) (n int, err error) {
	return r.pipe.read(data)
}

// ReadItem reads and returns a single T item from the pipe.
func (r *PipeReader[T]) ReadItem() (item T, err error) {
	var buf [1]T
	_, err = ReadFull(r, buf[:])
	return buf[0], err
}

// Close closes the reader; subsequent writes to the
// write half of the pipe will return the error ErrClosedPipe.
func (r *PipeReader[T]) Close() error {
	return r.CloseWithError(nil)
}

// CloseWithError closes the reader; subsequent writes
// to the write half of the pipe will return the error err.
//
// CloseWithError never overwrites the previous error if it exists
// and always returns nil.
func (r *PipeReader[T]) CloseWithError(err error) error {
	return r.pipe.closeRead(err)
}

// A PipeWriter is the write half of a pipe.
type PipeWriter[T any] struct {
	r PipeReader[T]
}

// Write implements the standard Write interface:
// it writes data to the pipe, blocking until one or more readers
// have consumed all the data or the read end is closed.
// If the read end is closed with an error, that err is
// returned as err; otherwise err is ErrClosedPipe.
func (w *PipeWriter[T]) Write(data []T) (n int, err error) {
	return w.r.pipe.write(data)
}

// WriteItem writes a single T item to the pipe.
func (w *PipeWriter[T]) WriteItem(item T) error {
	buf := [1]T{item}
	_, err := w.Write(buf[:])
	return err
}

// Close closes the writer; subsequent reads from the
// read half of the pipe will return no T items and EOF.
func (w *PipeWriter[T]) Close() error {
	return w.CloseWithError(nil)
}

// CloseWithError closes the writer; subsequent reads from the
// read half of the pipe will return no T items and the error err,
// or EOF if err is nil.
//
// CloseWithError never overwrites the previous error if it exists
// and always returns nil.
func (w *PipeWriter[T]) CloseWithError(err error) error {
	return w.r.pipe.closeWrite(err)
}

// Pipe creates a synchronous in-memory pipe.
// It can be used to connect code expecting an io.Reader
// with code expecting an io.Writer.
//
// Reads and Writes on the pipe are matched one to one
// except when multiple Reads are needed to consume a single Write.
// That is, each Write to the PipeWriter blocks until it has satisfied
// one or more Reads from the PipeReader that fully consume
// the written data.
// The data is copied directly from the Write to the corresponding
// Read (or Reads); there is no internal buffering.
//
// It is safe to call Read and Write in parallel with each other or with Close.
// Parallel calls to Read and parallel calls to Write are also safe:
// the individual calls will be gated sequentially.
func Pipe[T any]() (*PipeReader[T], *PipeWriter[T]) {
	pw := &PipeWriter[T]{r: PipeReader[T]{pipe: pipe[T]{
		wrCh: make(chan []T),
		rdCh: make(chan int),
		done: make(chan struct{}),
	}}}
	return &pw.r, pw
}

// BufferedPipe creates a bounded asynchronous in-memory pipe with a ring buffer
// of capacity size T items. If size <= 0, it behaves identically to [Pipe].
//
// Writes to the PipeWriter do not block as long as the internal buffer has
// sufficient capacity. Once the buffer is full, Write blocks until a Read
// frees capacity or the PipeReader is closed.
//
// When the PipeWriter is closed, any buffered items remain available to Read
// before EOF (or the close error) is returned.
func BufferedPipe[T any](size int) (*PipeReader[T], *PipeWriter[T]) {
	if size <= 0 {
		return Pipe[T]()
	}
	bp := &bufferedPipe[T]{
		buf: make([]T, size),
	}
	bp.notEmpty = sync.NewCond(&bp.mu)
	bp.notFull = sync.NewCond(&bp.mu)

	pw := &PipeWriter[T]{r: PipeReader[T]{pipe: pipe[T]{
		bp: bp,
	}}}
	return &pw.r, pw
}

type bufferedPipe[T any] struct {
	wrMu     sync.Mutex
	mu       sync.Mutex
	notEmpty *sync.Cond
	notFull  *sync.Cond

	buf     []T
	head    int
	count   int
	rClosed bool
	wClosed bool
	rerr    error
	werr    error
}

func (bp *bufferedPipe[T]) read(b []T) (n int, err error) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	for {
		if bp.rClosed {
			return 0, bp.readCloseErrorLocked()
		}
		if bp.count > 0 {
			if len(b) == 0 {
				return 0, nil
			}
			for n < len(b) && bp.count > 0 {
				avail := min(bp.count, len(bp.buf)-bp.head, len(b)-n)
				copy(b[n:n+avail], bp.buf[bp.head:bp.head+avail])
				clear(bp.buf[bp.head : bp.head+avail])
				bp.head = (bp.head + avail) % len(bp.buf)
				bp.count -= avail
				n += avail
			}
			bp.notFull.Broadcast()
			return n, nil
		}
		if bp.wClosed {
			return 0, bp.readCloseErrorLocked()
		}
		bp.notEmpty.Wait()
	}
}

func (bp *bufferedPipe[T]) write(b []T) (n int, err error) {
	bp.mu.Lock()
	if bp.rClosed || bp.wClosed {
		err = bp.writeCloseErrorLocked()
		bp.mu.Unlock()
		return 0, err
	}
	bp.mu.Unlock()

	bp.wrMu.Lock()
	defer bp.wrMu.Unlock()

	bp.mu.Lock()
	defer bp.mu.Unlock()

	for once := true; once || len(b) > 0; once = false {
		for bp.count == len(bp.buf) && len(b) > 0 && !bp.rClosed && !bp.wClosed {
			bp.notFull.Wait()
		}
		if bp.rClosed || bp.wClosed {
			return n, bp.writeCloseErrorLocked()
		}
		if len(b) == 0 {
			return n, nil
		}
		for len(b) > 0 && bp.count < len(bp.buf) {
			tail := (bp.head + bp.count) % len(bp.buf)
			space := min(len(bp.buf)-bp.count, len(bp.buf)-tail, len(b))
			copy(bp.buf[tail:tail+space], b[:space])
			bp.count += space
			b = b[space:]
			n += space
		}
		bp.notEmpty.Broadcast()
	}
	return n, nil
}

func (bp *bufferedPipe[T]) closeRead(err error) error {
	if err == nil {
		err = ErrClosedPipe
	}
	bp.mu.Lock()
	defer bp.mu.Unlock()
	if bp.rerr == nil {
		bp.rerr = err
	}
	bp.rClosed = true
	clear(bp.buf)
	bp.count = 0
	bp.head = 0
	bp.notEmpty.Broadcast()
	bp.notFull.Broadcast()
	return nil
}

func (bp *bufferedPipe[T]) closeWrite(err error) error {
	if err == nil {
		err = EOF
	}
	bp.mu.Lock()
	defer bp.mu.Unlock()
	if bp.werr == nil {
		bp.werr = err
	}
	bp.wClosed = true
	bp.notEmpty.Broadcast()
	bp.notFull.Broadcast()
	return nil
}

func (bp *bufferedPipe[T]) readCloseErrorLocked() error {
	if bp.rerr == nil && bp.werr != nil {
		return bp.werr
	}
	return ErrClosedPipe
}

func (bp *bufferedPipe[T]) writeCloseErrorLocked() error {
	if bp.werr == nil && bp.rerr != nil {
		return bp.rerr
	}
	return ErrClosedPipe
}
