// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build darwin

package portlist

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strings"

	"go.uber.org/atomic"
	"go4.org/mem"
)

// init initializes the Poller by ensuring it has an underlying
func (p *Poller) init() {
	p.os = newMacOSImpl(p.IncludeLocalhost)
}

type macOSImpl struct {
	known       map[protoPort]*portMeta // inode string => metadata
	netstatPath string                  // lazily populated

	br               *bufio.Reader // reused
	portsBuf         []Port
	includeLocalhost bool
}

type protoPort struct {
	proto string
	port  uint16
}

type portMeta struct {
	port Port
	keep bool
}

func newMacOSImpl(includeLocalhost bool) osImpl {
	return &macOSImpl{
		known:            map[protoPort]*portMeta{},
		br:               bufio.NewReader(bytes.NewReader(nil)),
		includeLocalhost: includeLocalhost,
	}
}

func (*macOSImpl) Close() error { return nil }

func (im *macOSImpl) AppendListeningPorts(base []Port) ([]Port, error) {
	var err error
	im.portsBuf, err = im.appendListeningPortsNetstat(im.portsBuf[:0])
	if err != nil {
		return nil, err
	}

	for _, pm := range im.known {
		pm.keep = false
	}

	var needProcs bool
	for _, p := range im.portsBuf {
		fp := protoPort{
			proto: p.Proto,
			port:  p.Port,
		}
		if pm, ok := im.known[fp]; ok {
			pm.keep = true
		} else {
			needProcs = true
			im.known[fp] = &portMeta{
				port: p,
				keep: true,
			}
		}
	}

	ret := base
	for k, m := range im.known {
		if !m.keep {
			delete(im.known, k)
		}
	}

	if needProcs {
		_ = im.addProcesses() // best effort
	}

	for _, m := range im.known {
		ret = append(ret, m.port)
	}
	return sortAndDedup(ret), nil
}

func (im *macOSImpl) appendListeningPortsNetstat(base []Port) ([]Port, error) {
	if im.netstatPath == "" {
		var err error
		im.netstatPath, err = exec.LookPath("netstat")
		if err != nil {
			return nil, fmt.Errorf("netstat: lookup: %v", err)
		}
	}

	cmd := exec.Command(im.netstatPath, "-na")
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	im.br.Reset(outPipe)

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer cmd.Process.Wait() //nolint:errcheck
	defer cmd.Process.Kill() //nolint:errcheck

	return appendParsePortsNetstat(base, im.br, im.includeLocalhost)
}

var lsofFailed atomic.Bool

// In theory, lsof could replace the function of both listPorts() and
// addProcesses(), since it provides a superset of the netstat output.
// However, "netstat -na" runs ~100x faster than lsof on my machine, so
// we should do it only if the list of open ports has actually changed.
//
// This fails in a macOS sandbox (i.e. in the Mac App Store or System
// Extension GUI build), but does at least work in the
// tailscaled-on-macos mode.
func (im *macOSImpl) addProcesses() error {
	if lsofFailed.Load() {
		// This previously failed in the macOS sandbox, so don't try again.
		return nil
	}
	exe, err := exec.LookPath("lsof")
	if err != nil {
		return fmt.Errorf("lsof: lookup: %v", err)
	}
	lsofCmd := exec.Command(exe, "-F", "-n", "-P", "-O", "-S2", "-T", "-i4", "-i6")
	outPipe, err := lsofCmd.StdoutPipe()
	if err != nil {
		return err
	}
	err = lsofCmd.Start()
	if err != nil {
		var stderr []byte
		if xe, ok := err.(*exec.ExitError); ok {
			stderr = xe.Stderr
		}
		// fails when run in a macOS sandbox, so make this non-fatal.
		if lsofFailed.CompareAndSwap(false, true) {
			log.Printf("portlist: can't run lsof in Mac sandbox; omitting process names from service list. Error details: %v, %s", err, bytes.TrimSpace(stderr))
		}
		return nil
	}
	defer func() {
		ps, err := lsofCmd.Process.Wait()
		if err != nil || ps.ExitCode() != 0 {
			log.Printf("portlist: can't run lsof in Mac sandbox; omitting process names from service list. Error: %v, exit code %d", err, ps.ExitCode())
			lsofFailed.Store(true)
		}
	}()
	defer lsofCmd.Process.Kill() //nolint:errcheck

	im.br.Reset(outPipe)

	var cmd, proto string
	var pid int
	for {
		line, err := im.br.ReadBytes('\n')
		if err != nil {
			break
		}
		if len(line) < 1 {
			continue
		}
		field, val := line[0], bytes.TrimSpace(line[1:])
		switch field {
		case 'p':
			// starting a new process
			cmd = ""
			proto = ""
			pid = 0
			if p, err := mem.ParseInt(mem.B(val), 10, 0); err == nil {
				pid = int(p)
			}
		case 'c':
			cmd = string(val) // TODO(bradfitz): avoid garbage; cache process names between runs?
		case 'P':
			proto = lsofProtoLower(val)
		case 'n':
			if mem.Contains(mem.B(val), mem.S("->")) {
				continue
			}
			// a listening port
			port := parsePort(mem.B(val))
			if port <= 0 {
				continue
			}
			pp := protoPort{proto, uint16(port)}
			m := im.known[pp]
			switch {
			case m != nil:
				m.port.Process = cmd
				m.port.Pid = pid
			default:
				// ignore: processes and ports come and go
			}
		}
	}

	return nil
}

func lsofProtoLower(p []byte) string {
	if string(p) == "TCP" {
		return "tcp"
	}
	if string(p) == "UDP" {
		return "udp"
	}
	return strings.ToLower(string(p))
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

func (a List) String() string {
	var sb strings.Builder
	for _, v := range a {
		fmt.Fprintf(&sb, "%-3s %5d %#v\n",
			v.Proto, v.Port, v.Process)
	}
	return strings.TrimRight(sb.String(), "\n")
}

// sortAndDedup sorts ps in place (by Port.LessThan) and then returns
// a subset of it with duplicate (Proto, Port) removed.
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
