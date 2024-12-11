// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

// This file contains the code related to the Poller type and its methods.
// The hot loop to keep efficient is Poller.Run.

package portlist

import (
	"fmt"
	"sort"
	"sync"

	"golang.org/x/exp/slices"
)

// Poller scans the systems for listening ports periodically and sends
// the results to C.
type Poller struct {
	// IncludeLocalhost controls whether services bound to localhost are included.
	//
	// This field should only be changed before calling Run.
	IncludeLocalhost bool

	// os, if non-nil, is an OS-specific implementation of the portlist getting
	// code. When non-nil, it's responsible for getting the complete list of
	// cached ports complete with the process name. That is, when set,
	// addProcesses is not used.
	// A nil values means we don't have code for getting the list on the current
	// operating system.
	os       osImpl
	initOnce sync.Once // guards init of os
	initErr  error

	// scatch is memory for Poller.getList to reuse between calls.
	scratch []Port

	prev List // most recent data, not aliasing scratch
}

// osImpl is the OS-specific implementation of getting the open listening ports.
type osImpl interface {
	Close() error

	// AppendListeningPorts appends to base (which must have length 0 but
	// optional capacity) the list of listening ports. The Port struct should be
	// populated as completely as possible. Another pass will not add anything
	// to it.
	//
	// The appended ports should be in a sorted (or at least stable) order so
	// the caller can cheaply detect when there are no changes.
	AppendListeningPorts(base []Port) ([]Port, error)
}

func (p *Poller) setPrev(pl List) {
	// Make a copy, as the pass in pl slice aliases pl.scratch and we don't want
	// that to except to the caller.
	p.prev = slices.Clone(pl)
}

// Close closes the Poller.
func (p *Poller) Close() error {
	if p.initErr != nil {
		return p.initErr
	}
	if p.os == nil {
		return nil
	}
	return p.os.Close()
}

// Poll returns the list of listening ports, if changed from
// a previous call as indicated by the changed result.
func (p *Poller) Poll() (ports []Port, changed bool, err error) {
	p.initOnce.Do(p.init)
	if p.initErr != nil {
		return nil, false, fmt.Errorf("error initializing poller: %w", p.initErr)
	}
	pl, err := p.getList()
	if err != nil {
		return nil, false, err
	}
	if pl.equal(p.prev) {
		return nil, false, nil
	}
	p.setPrev(pl)
	return p.prev, true, nil
}

func (p *Poller) getList() (List, error) {
	var err error
	p.scratch, err = p.os.AppendListeningPorts(p.scratch[:0])
	return p.scratch, err
}

func (a *Port) lessThan(b *Port) bool {
	if a.Port != b.Port {
		return a.Port < b.Port
	}
	if a.Proto != b.Proto {
		return a.Proto < b.Proto
	}
	return a.Process < b.Process
}

// sortAndDedup sorts ps in place (by Port.LessThan) and then returns
// a subset of it with duplicate (Proto, Port) removed.
// Multiple processes can't use the same port and protocol
// on the same port so there's no need to check for that
func sortAndDedup(ps List) List {
	sort.Slice(ps, func(i, j int) bool {
		return (&ps[i]).lessThan(&ps[j])
	})
	out := ps[:0]
	var last Port
	for _, p := range ps {
		if last.Proto == p.Proto && last.Port == p.Port {
			continue
		}
		out = append(out, p)
		last = p
	}
	return out
}
