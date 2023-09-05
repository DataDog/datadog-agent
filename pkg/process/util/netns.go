// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package util

import (
	"errors"
	"fmt"
	"os"
	"path"
	"runtime"
	"syscall"

	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// WithRootNS executes a function within root network namespace and then switch back
// to the previous namespace. If the thread is already in the root network namespace,
// the function is executed without calling SYS_SETNS.
func WithRootNS(procRoot string, fn func() error) error {
	rootNS, err := GetRootNetNamespace(procRoot)
	if err != nil {
		return err
	}
	defer rootNS.Close()

	return WithNS(rootNS, fn)
}

// WithNS executes the given function in the given network namespace, and then
// switches back to the previous namespace.
func WithNS(ns netns.NsHandle, fn func() error) error {
	if ns == netns.None() {
		return fn()
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	prevNS, err := netns.Get()
	if err != nil {
		return err
	}
	defer prevNS.Close()

	if ns.Equal(prevNS) {
		return fn()
	}

	if err := netns.Set(ns); err != nil {
		return err
	}

	fnErr := fn()
	nsErr := netns.Set(prevNS)
	if fnErr != nil {
		return fnErr
	}
	return nsErr
}

// GetNetNamespaces returns a list of network namespaces on the machine. The caller
// is responsible for calling Close() on each of the returned NsHandle's.
func GetNetNamespaces(procRoot string) ([]netns.NsHandle, error) {
	var nss []netns.NsHandle
	seen := make(map[string]interface{})
	err := WithAllProcs(procRoot, func(pid int) error {
		ns, err := netns.GetFromPath(path.Join(procRoot, fmt.Sprintf("%d/ns/net", pid)))
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, unix.ENOENT) {
				log.Errorf("error while reading %s: %s", path.Join(procRoot, fmt.Sprintf("%d/ns/net", pid)), err)
			}
			return nil
		}

		uid := ns.UniqueId()
		if _, ok := seen[uid]; ok {
			ns.Close()
			return nil
		}

		seen[uid] = struct{}{}
		nss = append(nss, ns)
		return nil
	})

	if err != nil {
		// close all the accumulated ns handles
		for _, ns := range nss {
			ns.Close()
		}

		return nil, err
	}

	return nss, nil
}

// GetCurrentIno returns the ino number for the current network namespace
func GetCurrentIno() (uint32, error) {
	curNS, err := netns.Get()
	if err != nil {
		return 0, err
	}
	defer curNS.Close()

	return GetInoForNs(curNS)
}

// GetRootNetNamespace gets the root network namespace
func GetRootNetNamespace(procRoot string) (netns.NsHandle, error) {
	return GetNetNamespaceFromPid(procRoot, 1)
}

// GetNetNamespaceFromPid gets the network namespace for a given `pid`
func GetNetNamespaceFromPid(procRoot string, pid int) (netns.NsHandle, error) {
	return netns.GetFromPath(path.Join(procRoot, fmt.Sprintf("%d/ns/net", pid)))
}

// GetNetNsInoFromPid gets the network namespace inode number for the given
// `pid`
func GetNetNsInoFromPid(procRoot string, pid int) (uint32, error) {
	ns, err := GetNetNamespaceFromPid(procRoot, pid)
	if err != nil {
		return 0, err
	}

	defer ns.Close()

	return GetInoForNs(ns)
}

// GetInoForNs gets the inode number for the given network namespace
func GetInoForNs(ns netns.NsHandle) (uint32, error) {
	if ns.Equal(netns.None()) {
		return 0, fmt.Errorf("net ns is none")
	}

	var s syscall.Stat_t
	if err := syscall.Fstat(int(ns), &s); err != nil {
		return 0, err
	}

	return uint32(s.Ino), nil
}
