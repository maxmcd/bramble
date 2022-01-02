package io2

import "io"

type writerMultiCloser struct {
	io.Writer
	closers []io.Closer
}

func (w writerMultiCloser) Close() error {
	for _, c := range w.closers {
		if err := c.Close(); err != nil {
			return err
		}
	}
	return nil
}

// WriterMultiCloser adds multiple closers to a writer. If close is called on
// the returned io.WriteCloser it will call all closers in order and return
// immediately if any errors are encountered
func WriterMultiCloser(w io.Writer, closers ...io.Closer) io.WriteCloser {
	return writerMultiCloser{Writer: w, closers: closers}
}

// ReaderMultiCloser adds multiple closers to a reader. If close is called on
// the returned io.ReadCloser it will call all closers in order and return
// immediately if any errors are encountered
func ReaderMultiCloser(r io.Reader, closers ...io.Closer) io.ReadCloser {
	return readerMultiCloser{Reader: r, closers: closers}
}

type readerMultiCloser struct {
	io.Reader
	closers []io.Closer
}

func (w readerMultiCloser) Close() error {
	for _, c := range w.closers {
		if err := c.Close(); err != nil {
			return err
		}
	}
	return nil
}

func WriterCloseFunc(w io.Writer, close func() error) io.WriteCloser {
	return &writerCloseFunc{Writer: w, close: close}
}

type writerCloseFunc struct {
	io.Writer
	close func() error
}

func (w writerCloseFunc) Close() error { return w.close() }
