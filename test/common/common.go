package common

// #include <stdio.h>
// #include <stdlib.h>
import "C"
import (
	"bytes"
	"io"
	"os"
	"sync"
	"syscall"
)

var lock sync.Mutex

// Capture code from https://github.com/zimmski/osutil
func Capture(call func()) (output []byte, err error) {
	lock.Lock()
	defer lock.Unlock()

	originalStdout, e := syscall.Dup(syscall.Stdout)
	if e != nil {
		return nil, e
	}

	originalStderr, e := syscall.Dup(syscall.Stderr)
	if e != nil {
		return nil, e
	}

	defer func() {
		if e := syscall.Dup2(originalStdout, syscall.Stdout); e != nil {
			err = e
		}
		if e := syscall.Close(originalStdout); e != nil {
			err = e
		}
		if e := syscall.Dup2(originalStderr, syscall.Stderr); e != nil {
			err = e
		}
		if e := syscall.Close(originalStderr); e != nil {
			err = e
		}
	}()

	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer func() {
		e := r.Close()
		if e != nil {
			err = e
		}
	}()

	if e := syscall.Dup2(int(w.Fd()), syscall.Stdout); e != nil {
		return nil, e
	}
	if e := syscall.Dup2(int(w.Fd()), syscall.Stderr); e != nil {
		return nil, e
	}

	out := make(chan []byte)
	go func() {
		var b bytes.Buffer

		_, err := io.Copy(&b, r)
		if err != nil {
			panic(err)
		}

		out <- b.Bytes()
	}()

	call()

	C.fflush(C.stdout)

	err = w.Close()
	if err != nil {
		return nil, err
	}
	if e := syscall.Close(syscall.Stdout); e != nil {
		return nil, e
	}
	if e := syscall.Close(syscall.Stderr); e != nil {
		return nil, e
	}

	return <-out, err
}
