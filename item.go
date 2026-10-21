package gio

import (
	"errors"
	"io"
)

// ErrInvalidUnread means that UnreadItem was called when the preceding
// operation was not a successful ReadItem call or when UnreadItem was
// already called without an intervening ReadItem.
var ErrInvalidUnread = errors.New("gio: invalid use of UnreadItem")

// ItemReader is the interface that wraps the ReadItem method.
//
// ReadItem reads and returns the next item from the input or
// any error encountered. If ReadItem returns an error, no input
// item was consumed, and the returned item value is undefined.
//
// ReadItem provides an efficient interface for item-at-a-time
// processing. A Reader that does not implement ItemReader
// can be wrapped using NewItemScanner or called via ReadItem.
type ItemReader[T any] interface {
	ReadItem() (T, error)
}

// ItemScanner is the interface that adds the UnreadItem method to the
// basic ReadItem method.
//
// UnreadItem causes the next call to ReadItem to return the last item read.
// If the last operation was not a successful call to ReadItem, UnreadItem may
// return an error, unread the last item read (or the item prior to the
// last-unread item), or (in implementations that support the Seeker interface)
// seek to one item before the current offset.
type ItemScanner[T any] interface {
	ItemReader[T]
	UnreadItem() error
}

// ReadItemScanner is the interface that groups the basic Read, ReadItem,
// and UnreadItem methods.
type ReadItemScanner[T any] interface {
	Reader[T]
	ItemScanner[T]
}

// ItemWriter is the interface that wraps the WriteItem method.
type ItemWriter[T any] interface {
	WriteItem(item T) error
}

// ReadItem reads and returns a single T item from r.
// If r implements [ItemReader], ReadItem calls r.ReadItem().
// If T is byte and r implements [ByteReader], ReadItem calls r.ReadByte().
// Otherwise, ReadItem reads a single item using [ReadFull].
func ReadItem[T any](r Reader[T]) (item T, err error) {
	if ir, ok := r.(ItemReader[T]); ok {
		return ir.ReadItem()
	}
	if br, ok := any(r).(io.ByteReader); ok {
		b, err := br.ReadByte()
		return any(b).(T), err
	}
	var buf [1]T
	_, err = ReadFull(r, buf[:])
	return buf[0], err
}

// WriteItem writes a single T item to w.
// If w implements [ItemWriter], WriteItem calls w.WriteItem(item).
// If T is byte and w implements [ByteWriter], WriteItem calls w.WriteByte(item).
// Otherwise, WriteItem writes a 1-element slice to w.
func WriteItem[T any](w Writer[T], item T) error {
	if iw, ok := w.(ItemWriter[T]); ok {
		return iw.WriteItem(item)
	}
	if bw, ok := any(w).(io.ByteWriter); ok {
		return bw.WriteByte(any(item).(byte))
	}
	buf := [1]T{item}
	n, err := w.Write(buf[:])
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrShortWrite
	}
	return nil
}

// WriteItems writes one or more T items to w.
func WriteItems[T any](w Writer[T], items ...T) (n int, err error) {
	if len(items) == 0 {
		return 0, nil
	}
	return w.Write(items)
}

// NewItemScanner wraps r to provide [ReadItemScanner] (Read, ReadItem, and UnreadItem).
// If r already implements [ReadItemScanner], it is returned directly.
func NewItemScanner[T any](r Reader[T]) ReadItemScanner[T] {
	if ris, ok := r.(ReadItemScanner[T]); ok {
		return ris
	}
	return &itemScanner[T]{r: r}
}

type itemScanner[T any] struct {
	r       Reader[T]
	last    T
	hasLast bool
	unread  bool
}

func (s *itemScanner[T]) Read(p []T) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	if s.unread {
		p[0] = s.last
		s.unread = false
		s.hasLast = false
		return 1, nil
	}
	n, err = s.r.Read(p)
	if n > 0 {
		s.last = p[n-1]
		s.hasLast = true
	}
	return n, err
}

func (s *itemScanner[T]) ReadItem() (T, error) {
	if s.unread {
		s.unread = false
		return s.last, nil
	}
	item, err := ReadItem(s.r)
	if err != nil {
		var zero T
		return zero, err
	}
	s.last = item
	s.hasLast = true
	return item, nil
}

func (s *itemScanner[T]) UnreadItem() error {
	if !s.hasLast || s.unread {
		return ErrInvalidUnread
	}
	s.unread = true
	return nil
}
