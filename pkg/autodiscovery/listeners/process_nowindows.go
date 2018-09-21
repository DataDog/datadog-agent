// +build !windows

package listeners

import (
	"fmt"

	"github.com/shirou/gopsutil/net"
)

func getProcessPorts(pid int32) ([]int, error) {
	conns, err := net.ConnectionsPid("all", pid)
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
