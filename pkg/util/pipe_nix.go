// +build darwin freebsd linux

package util

import (
	"errors"
	"os"
	"syscall"
)

type UnixNamedPipe struct {
	path string
	pipe *os.File
}

func GetPipe(path string) (NamedPipe, error) {
	return NewUnixNamedPipe(path)
}

func NewUnixNamedPipe(path string) (*UnixNamedPipe, error) {
	return &UnixNamedPipe{path: path}, nil
}

func (p *UnixNamedPipe) Open() error {
	var err error
	if p.pipe != nil {
		//open pipe
		return nil
	}

	if _, err := os.Stat(p.path); os.IsNotExist(err) {
		//Create the pipe
		err = syscall.Mkfifo(p.path, 0600)
		if err != nil {
			return err
		}

	}

	p.pipe, err = os.OpenFile(p.path, os.O_RDWR, 0600)
	if err != nil {
		return err
	}

	return nil
}

func (p *UnixNamedPipe) Ready() bool {
	return (p.pipe != nil)
}

func (p *UnixNamedPipe) Read(b []byte) (int, error) {
	if p.pipe == nil {
		return 0, errors.New("No pipe to write to.")
	}
	return p.pipe.Read(b)
}

func (p *UnixNamedPipe) Write(b []byte) (int, error) {
	if p.pipe == nil {
		return 0, nil
	}
	return p.pipe.Write(b)
}

func (p *UnixNamedPipe) Close() error {
	if p.pipe == nil {
		return nil
	}

	err := p.pipe.Close()
	if err != nil {
		return err
	}

	p.pipe = nil
	return err
}
