//+build linux

package netlink

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/vishvananda/netns"
)

// getGlobalNetNSFD returns the file descriptor of the root net NS
// and a boolean indicating whether it matches the current namespace.
// In case of error, (0, false) is returned.
func getGlobalNetNSFD(procRoot string) (fd int, inRootNS bool) {
	var err error
	path := fmt.Sprintf("%s/1/ns/net", procRoot)
	defer func() {
		if err != nil {
			log.Warnf("could not attach to net namespace at %s: %v", path, err)
		}
	}()

	currentNS, err := netns.Get()
	if err != nil {
		return 0, false
	}

	rootNS, err := netns.GetFromPath(path)
	if err != nil {
		return 0, false
	}

	log.Infof("attaching to net namespace at %s", path)
	return int(rootNS), rootNS.Equal(currentNS)
}
