package docker

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
	"github.com/docker/docker/client"
)

// GetHostname queries Docker for the host name
func GetHostname() (string, error) {
	client, err := client.NewEnvClient()
	if err != nil {
		return "", err
	}
	defer client.Close()

	info, err := client.Info(context.Background())
	if err != nil {
		return "", fmt.Errorf("unable to get Docker info: %s", err)
	}

	return info.Name, nil
}

// HostnameProvider docker implementation for the hostname provider
func HostnameProvider(hostName string) (string, error) {
	return GetHostname()
}

// DefaultGateway returns the default Docker gateway.
func DefaultGateway() (net.IP, error) {
	procRoot := config.Datadog.GetString("proc_root")
	netRouteFile := filepath.Join(procRoot, "net", "route")
	f, err := os.Open(netRouteFile)
	if os.IsNotExist(err) || os.IsPermission(err) {
		log.Errorf("unable to open %s: %s", netRouteFile, err)
		return nil, nil
	} else if err != nil {
		// Unknown error types will bubble up for handling.
		return nil, err
	}
	defer f.Close()

	ip := make(net.IP, 4)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 && fields[1] == "00000000" {
			ipInt, err := strconv.ParseInt(fields[2], 16, 32)
			if err != nil {
				return nil, fmt.Errorf("unable to parse ip %s, from %s: %s", fields[2], netRouteFile, err)
			}
			binary.LittleEndian.PutUint32(ip, uint32(ipInt))
			break
		}
	}
	return ip, nil
}
