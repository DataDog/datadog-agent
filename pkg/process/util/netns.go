// +build linux

package util

import (
	"fmt"
	"runtime"

	"github.com/vishvananda/netns"
)

// WithRootNS executes a function within root network namespace and then switch back
// to the previous namespace. If the thread is already in the root network namespace,
// the function is executed without calling SYS_SETNS.
func WithRootNS(procRoot string, fn func()) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	prevNS, err := netns.Get()
	if err != nil {
		return err
	}

	rootNS, err := netns.GetFromPath(fmt.Sprintf("%s/1/ns/net", procRoot))
	if err != nil {
		return err
	}

	if rootNS.Equal(prevNS) {
		fn()
		return nil
	}

	if err := netns.Set(rootNS); err != nil {
		return err
	}

	fn()
	return netns.Set(prevNS)
}
