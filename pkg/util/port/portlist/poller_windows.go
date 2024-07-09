// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package portlist

import (
	"errors"
)

// ErrNotImplemented is the "not implemented" error given by `gopsutil` when an
// OS doesn't support and API. Unfortunately it's in an internal package so
// we can't import it so we'll copy it here.
var ErrNotImplemented = errors.New("not implemented yet")

// init initializes the Poller by ensuring it has an underlying
func (p *Poller) init() {
	p.os = newWindowsImpl(p.IncludeLocalhost)
}

type windowsImpl struct {
	known            map[famPort]*portMeta
	includeLocalhost bool
}

type famPort struct {
	proto string
	port  uint16
	pid   uint32
}

type portMeta struct {
	port Port
	keep bool
}

func newWindowsImpl(includeLocalhost bool) osImpl {
	return &windowsImpl{
		known:            map[famPort]*portMeta{},
		includeLocalhost: includeLocalhost,
	}
}
func (*windowsImpl) Close() error { return nil }

func (im *windowsImpl) AppendListeningPorts(base []Port) ([]Port, error) {
	// TODO(bradfitz): netstat.Get makes a bunch of garbage, add an append-style API to that package instead/additionally
	// tab, err :=netstat.Get()
	tab, err := GetConnTable()
	if err != nil {
		return nil, err
	}

	for _, pm := range im.known {
		pm.keep = false
	}

	ret := base
	for _, e := range tab.Entries {
		if e.State != "LISTEN" {
			continue
		}
		if !im.includeLocalhost && !e.Local.Addr().IsUnspecified() {
			continue
		}
		fp := famPort{
			proto: "tcp", // TODO(bradfitz): UDP too; add to netstat
			port:  e.Local.Port(),
			pid:   uint32(e.Pid),
		}
		pm, ok := im.known[fp]
		if ok {
			pm.keep = true
			continue
		}
		var process string
		if e.OSMetadata != nil {
			if module, err := e.OSMetadata.GetModule(); err == nil {
				process = module
			}
		}
		pm = &portMeta{
			keep: true,
			port: Port{
				Proto:   "tcp",
				Port:    e.Local.Port(),
				Process: process,
				Pid:     e.Pid,
			},
		}
		im.known[fp] = pm
	}

	for k, m := range im.known {
		if !m.keep {
			delete(im.known, k)
			continue
		}
		ret = append(ret, m.port)
	}

	return sortAndDedup(ret), nil
}
