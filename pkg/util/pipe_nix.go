// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin || freebsd || linux

package util

import (
	"errors"
	"os"
	"syscall"
)

// UnixNamedPipe unix abstraction to named pipes
type UnixNamedPipe struct {
	path string
	pipe *os.File
}

// GetPipe returns a UnixPipe to path
func GetPipe(path string) (NamedPipe, error) {
	return NewUnixNamedPipe(path)
}

// NewUnixNamedPipe UnixNamedPipe constructor
func NewUnixNamedPipe(path string) (*UnixNamedPipe, error) {
	return &UnixNamedPipe{path: path}, nil
}

// Open opens named pipe - will create it if doesn't exist
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

// Ready is the pipe ready to read/write?
func (p *UnixNamedPipe) Ready() bool {
	return (p.pipe != nil)
}

// Read from the pipe
func (p *UnixNamedPipe) Read(b []byte) (int, error) {
	if p.pipe == nil {
		return 0, errors.New("no pipe to write to")
	}
	return p.pipe.Read(b)
}

// Write to the pipe
func (p *UnixNamedPipe) Write(b []byte) (int, error) {
	if p.pipe == nil {
		return 0, nil
	}
	return p.pipe.Write(b)
}

// Close the underlying named pipe
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
