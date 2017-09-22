// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package util

import (
	"errors"
	"net"

	"github.com/Microsoft/go-winio"
)

type WinNamedPipe struct {
	path string
	pipe net.Conn
}

func GetPipe(path string) (NamedPipe, error) {
	return NewWinNamedPipe(path)
}

func NewWinNamedPipe(path string) (*WinNamedPipe, error) {
	return &WinNamedPipe{path: path}, nil
}

func (p *WinNamedPipe) Open() error {
	if p.pipe != nil {
		return nil
	}

	c, err := winio.DialPipe(p.path, nil)
	if err != nil {
		//Create the pipe
		cfg := &winio.PipeConfig{MessageMode: true}
		l, err := winio.ListenPipe(p.path, cfg)
		if err != nil {
			return err
		}
		defer l.Close()
	}

	if c, err = winio.DialPipe(p.path, nil); err != nil {
		return err
	}
	p.pipe = c

	return nil
}

func (p *WinNamedPipe) Ready() bool {
	return (p.pipe != nil)
}

func (p *WinNamedPipe) Read(b []byte) (int, error) {
	if p.pipe == nil {
		return 0, errors.New("No pipe to write to.")
	}
	return p.pipe.Read(b)
}

func (p *WinNamedPipe) Write(b []byte) (int, error) {
	if p.pipe == nil {
		return 0, nil
	}
	return p.pipe.Write(b)
}

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
