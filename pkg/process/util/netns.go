// +build linux

package util

import (
	"fmt"
	"io/ioutil"
	"runtime"
	"strconv"

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
// is responsible for calling Close() on ech of the returned NsHandle's.
func GetNetNamespaces(procRoot string) ([]netns.NsHandle, error) {
	files, err := ioutil.ReadDir(procRoot)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]interface{})
	var nss []netns.NsHandle
	for _, f := range files {
		if !f.IsDir() {
			continue
		}

		if _, err := strconv.Atoi(f.Name()); err != nil {
			continue
		}

		ns, err := netns.GetFromPath(fmt.Sprintf("%s/%s/ns/net", procRoot, f.Name()))
		if err != nil {
			return nil, err
		}

		uid := ns.UniqueId()
		if _, ok := seen[uid]; ok {
			ns.Close()
			continue
		}

		seen[uid] = struct{}{}
		nss = append(nss, ns)
	}

	return nss, nil
}
