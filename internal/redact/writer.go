package redact

import (
	"bytes"
	"io"
	"sync"
)

type Writer struct {
	out      io.Writer
	redactor *Redactor
	lock     sync.Mutex
	pending  []byte
}

func NewWriter(out io.Writer, redactor *Redactor) *Writer {
	return &Writer{out: out, redactor: redactor}
}

func (w *Writer) Write(data []byte) (int, error) {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.pending = append(w.pending, data...)
	end := bytes.LastIndexByte(w.pending, '\n')
	if end < 0 {
		return len(data), nil
	}
	lines := string(w.pending[:end+1])
	w.pending = append([]byte{}, w.pending[end+1:]...)
	if _, err := io.WriteString(w.out, w.redactor.Redact(lines)); err != nil {
		return 0, err
	}
	return len(data), nil
}

func (w *Writer) Flush() error {
	w.lock.Lock()
	defer w.lock.Unlock()
	if len(w.pending) == 0 {
		return nil
	}
	_, err := io.WriteString(w.out, w.redactor.Redact(string(w.pending)))
	w.pending = nil
	return err
}
