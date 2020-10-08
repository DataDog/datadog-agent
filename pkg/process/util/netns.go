// +build linux

package util

import (
	"fmt"
	"path"
	"runtime"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/vishvananda/netns"
)

// WithRootNS executes a function within root network namespace and then switch back
// to the previous namespace. If the thread is already in the root network namespace,
// the function is executed without calling SYS_SETNS.
func WithRootNS(procRoot string, fn func()) error {
	rootNS, err := netns.GetFromPath(fmt.Sprintf("%s/1/ns/net", procRoot))
	if err != nil {
		return err
	}

	return WithNS(procRoot, rootNS, fn)
}

// WithNS executes the given function in the given network namespace, and then
// switches back to the previous namespace.
func WithNS(procRoot string, ns netns.NsHandle, fn func()) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	prevNS, err := netns.Get()
	if err != nil {
		return err
	}

	if ns.Equal(prevNS) {
		fn()
		return nil
	}

	if err := netns.Set(ns); err != nil {
		return err
	}

	fn()
	return netns.Set(prevNS)
}

// GetNetNamespaces returns a list of network namespaces on the machine. The caller
// is responsible for calling Close() on each of the returned NsHandle's.
func GetNetNamespaces(procRoot string) ([]netns.NsHandle, error) {
	var nss []netns.NsHandle
	seen := make(map[string]interface{})
	err := WithAllProcs(procRoot, func(pid int) error {
		ns, err := netns.GetFromPath(path.Join(procRoot, fmt.Sprintf("%d/ns/net", pid)))
		if err != nil {
			log.Errorf("error while reading %s: %s", path.Join(procRoot, fmt.Sprintf("%d/ns/net", pid)), err)
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
func GetNetNsInoFromPid(procRoot string, pid int) (uint64, error) {
	ns, err := GetNetNamespaceFromPid(procRoot, pid)
	if err != nil {
		return 0, err
	}

	defer ns.Close()

	return GetInoForNs(ns)
}

// GetInoForNs gets the inode number for the given network namespace
func GetInoForNs(ns netns.NsHandle) (uint64, error) {
	if ns.Equal(netns.None()) {
		return 0, fmt.Errorf("net ns is none")
	}

	var s syscall.Stat_t
	if err := syscall.Fstat(int(ns), &s); err != nil {
		return 0, err
	}

	return s.Ino, nil
}
