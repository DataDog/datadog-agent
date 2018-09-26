// +build !windows

package listeners

import (
	"fmt"

	"github.com/shirou/gopsutil/net"
)

func getProcessUnixSockets(pid int32) ([]string, error) {
	conns, err := net.ConnectionsPid("unix", pid)
	if err != nil {
		return nil, fmt.Errorf("couldn't retrieve sockets for pid %d: %s", pid, err)
	}

	sockets := []string{}
	for _, conn := range conns {
		if len(conn.Laddr.IP) > 0 {
			sockets = append(sockets, conn.Laddr.IP)
		}
	}

	return sockets, nil
}

func getProcessPorts(pid int32) ([]int, error) {
	conns, err := net.ConnectionsPid("inet", pid)
	if err != nil {
		return nil, fmt.Errorf("couldn't retrieve ports for pid %d: %s", pid, err)
	}

	ports := []int{}
	for _, conn := range conns {
		if conn.Status != "NONE" {
			ports = append(ports, int(conn.Laddr.Port))
		}
	}

	return ports, nil
}
