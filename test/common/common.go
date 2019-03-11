// +build !windows

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

var lockStdFileDescriptorsSwapping sync.Mutex

// Capture code from https://github.com/zimmski/osutil
func Capture(call func()) (output []byte, err error) {
	lockStdFileDescriptorsSwapping.Lock()

	originalStdout, e := syscall.Dup(syscall.Stdout)
	if e != nil {
		lockStdFileDescriptorsSwapping.Unlock()

		return nil, e
	}

	originalStderr, e := syscall.Dup(syscall.Stderr)
	if e != nil {
		lockStdFileDescriptorsSwapping.Unlock()

		return nil, e
	}

	lockStdFileDescriptorsSwapping.Unlock()

	defer func() {
		lockStdFileDescriptorsSwapping.Lock()

		if e := syscall.Dup2(originalStdout, syscall.Stdout); e != nil {
			lockStdFileDescriptorsSwapping.Unlock()

			err = e
		}
		if e := syscall.Close(originalStdout); e != nil {
			lockStdFileDescriptorsSwapping.Unlock()

			err = e
		}
		if e := syscall.Dup2(originalStderr, syscall.Stderr); e != nil {
			lockStdFileDescriptorsSwapping.Unlock()

			err = e
		}
		if e := syscall.Close(originalStderr); e != nil {
			lockStdFileDescriptorsSwapping.Unlock()

			err = e
		}

		lockStdFileDescriptorsSwapping.Unlock()
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
		if w != nil {
			e = w.Close()
			if err != nil {
				err = e
			}
		}
	}()

	lockStdFileDescriptorsSwapping.Lock()

	if e := syscall.Dup2(int(w.Fd()), syscall.Stdout); e != nil {
		lockStdFileDescriptorsSwapping.Unlock()

		return nil, e
	}
	if e := syscall.Dup2(int(w.Fd()), syscall.Stderr); e != nil {
		lockStdFileDescriptorsSwapping.Unlock()

		return nil, e
	}

	lockStdFileDescriptorsSwapping.Unlock()

	out := make(chan []byte)
	go func() {
		defer func() {
			// If there is a panic in the function call, copying from "r" does not work anymore.
			_ = recover()
		}()

		var b bytes.Buffer

		_, err := io.Copy(&b, r)
		if err != nil {
			panic(err)
		}

		out <- b.Bytes()
	}()

	call()

	lockStdFileDescriptorsSwapping.Lock()

	C.fflush(C.stdout)

	err = w.Close()
	if err != nil {
		lockStdFileDescriptorsSwapping.Unlock()

		return nil, err
	}
	w = nil

	if e := syscall.Close(syscall.Stdout); e != nil {
		lockStdFileDescriptorsSwapping.Unlock()

		return nil, e
	}
	if e := syscall.Close(syscall.Stderr); e != nil {
		lockStdFileDescriptorsSwapping.Unlock()

		return nil, e
	}

	lockStdFileDescriptorsSwapping.Unlock()

	return <-out, err
}
