// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build linux

package portlist

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// initTCP installs tcpImpl in place of the default linuxImpl. It lives apart
// from linuxImpl.AppendListeningPorts so binaries that never call
// NewTCPPoller don't link it.
func (p *Poller) initTCP(listeners func() ([]TCPListener, error)) {
	p.initOnce.Do(func() {
		p.os = &tcpImpl{li: newLinuxImpl(p.IncludeLocalhost), listeners: listeners}
	})
}

// tcpImpl lists TCP listeners from a TCPListener source, falling back to
// /proc/net/tcp and tcp6. Both paths share li's inode cache.
type tcpImpl struct {
	li        *linuxImpl
	listeners func() ([]TCPListener, error)
	inoBuf    []byte // scratch space for "socket:[inode]" keys
}

func (t *tcpImpl) AppendListeningPorts(base []Port) ([]Port, error) {
	li := t.li
	defer li.br.Reset(eofReader)

	for _, pm := range li.known {
		pm.keep = false
	}

	listed := false
	if t.listeners != nil {
		if ls, err := t.listeners(); err == nil {
			t.addListeners(ls)
			listed = true
		}
	}
	if !listed {
		if err := t.parseProcNetTCP(); err != nil {
			return nil, err
		}
	}

	needProc := make(map[string]*portMeta)
	for inode, pm := range li.known {
		if !pm.keep {
			delete(li.known, inode)
			continue
		}
		if pm.needsProcName {
			needProc[inode] = pm
		}
	}
	if err := li.findProcessNames(needProc); err != nil {
		return nil, err
	}

	ret := base
	for _, pm := range li.known {
		ret = append(ret, pm.port)
	}
	return sortAndDedup(ret), nil
}

// addListeners marks or adds each listener in li.known, applying the same
// localhost filter as the /proc/net parser.
func (t *tcpImpl) addListeners(ls []TCPListener) {
	li := t.li
	for _, l := range ls {
		if !li.includeLocalhost && l.Loopback {
			continue
		}
		key := append(t.inoBuf[:0], "socket:["...)
		key = strconv.AppendUint(key, uint64(l.Inode), 10)
		key = append(key, ']')
		t.inoBuf = key
		if pm, ok := li.known[string(key)]; ok {
			pm.keep = true
			continue
		}
		li.known[string(key)] = &portMeta{
			needsProcName: true,
			keep:          true,
			port:          Port{Proto: "tcp", Port: l.Port, IP: l.IP},
		}
	}
}

func (t *tcpImpl) parseProcNetTCP() error {
	br := t.li.br
	for _, name := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		f, err := os.Open(name)
		if err != nil {
			if !os.IsNotExist(err) {
				log.Errorf("diagnose port-conflict poller warning; ignoring: %v", err)
			}
			continue
		}
		br.Reset(f)
		err = t.li.parseProcNetFile(br, filepath.Base(name))
		f.Close()
		if err != nil {
			return fmt.Errorf("parsing %q: %w", name, err)
		}
	}
	return nil
}
