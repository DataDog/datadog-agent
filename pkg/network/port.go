// +build linux

package network

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PortMapping represents a port binding
type PortMapping struct {
	Ino  uint32
	Port uint16
}

var statusMap = map[ConnectionType]int64{
	TCP: tcpListen,
	UDP: tcpClose,
}

// ReadInitialState reads the /proc filesystem and determines which ports are being listened on
func ReadInitialState(procRoot string, protocol ConnectionType, collectIPv6 bool) (map[PortMapping]struct{}, error) {
	start := time.Now()
	defer func() {
		log.Debugf("Read initial %s pid->port mapping in %s", protocol.String(), time.Now().Sub(start))
	}()

	lp := strings.ToLower(protocol.String())
	paths := []string{"net/" + lp}
	if collectIPv6 {
		paths = append(paths, "net/"+lp+"6")
	}

	status := statusMap[protocol]
	return readState(procRoot, paths, status)
}

func readState(procRoot string, paths []string, status int64) (map[PortMapping]struct{}, error) {
	seen := make(map[uint32]struct{})
	allports := make(map[PortMapping]struct{})
	err := util.WithAllProcs(procRoot, func(pid int) error {
		nsIno, err := util.GetNetNsInoFromPid(procRoot, pid)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				log.Errorf("error getting net ns for pid %d: %s", pid, err)
			}
			return nil
		}

		if _, ok := seen[nsIno]; ok {
			return nil
		}
		seen[nsIno] = struct{}{}

		for _, p := range paths {
			ports, err := readProcNetWithStatus(path.Join(procRoot, fmt.Sprintf("%d", pid), p), status)
			if err != nil {
				log.Errorf("error reading port state net ns ino=%d pid=%d path=%s status=%d", nsIno, pid, p, status)
				continue
			}

			for _, port := range ports {
				allports[PortMapping{Ino: nsIno, Port: port}] = struct{}{}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return allports, nil
}
