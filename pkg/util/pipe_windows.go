// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"errors"
	"net"

	"github.com/Microsoft/go-winio"
)

// WinNamedPipe holds the named pipe configuration
type WinNamedPipe struct {
	path string
	pipe net.Conn
}

// GetPipe returns a named pipe to path
func GetPipe(path string) (NamedPipe, error) {
	return &WinNamedPipe{path: path}, nil
}

// Open named pipe
func (p *WinNamedPipe) Open() error {
	if p.pipe != nil {
		return nil
	}

	_, err := winio.DialPipe(p.path, nil)
	if err != nil {
		//Create the pipe
		cfg := &winio.PipeConfig{MessageMode: true}
		l, err := winio.ListenPipe(p.path, cfg)
		if err != nil {
			return err
		}
		defer l.Close()
	}

	c, err := winio.DialPipe(p.path, nil)
	if err != nil {
		return err
	}
	p.pipe = c

	return nil
}

// Ready returns whether the pipe is ready to read and write
func (p *WinNamedPipe) Ready() bool {
	return (p.pipe != nil)
}

// Read from the pipe
func (p *WinNamedPipe) Read(b []byte) (int, error) {
	if p.pipe == nil {
		return 0, errors.New("no pipe to write to")
	}
	return p.pipe.Read(b)
}

// Write to the pipe
func (p *WinNamedPipe) Write(b []byte) (int, error) {
	if p.pipe == nil {
		return 0, nil
	}
	return p.pipe.Write(b)
}

// Close the underlying named pipe
func (p *WinNamedPipe) Close() error {
	var err error
	if p.pipe == nil {
		return nil
	}

	if err = p.pipe.Close(); err != nil {
		return err
	}

	p.pipe = nil
	return err
}
