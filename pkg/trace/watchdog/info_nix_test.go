// +build !windows

package watchdog

import "testing"

func TestNetHigh(t *testing.T) {
	doTestNetHigh(t, 10)
	if testing.Short() {
		return
	}
	doTestNetHigh(t, 200)
}
