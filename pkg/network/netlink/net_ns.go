//+build linux

package netlink

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// getGlobalNetNSFD guesses the file descriptor of the root net NS
// and returns 0 in case of failure
func getGlobalNetNSFD(procRoot string) int {
	path := procRoot + "/1/ns/net"
	file, err := os.Open(path)
	if err != nil {
		log.Warnf("could not attach to net namespace at %s: %v", path, err)
		return 0
	}

	log.Infof("attaching to net namespace at %s", path)
	return int(file.Fd())
}
