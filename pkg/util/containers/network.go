// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-2020 Datadog, Inc.

package containers

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DefaultGateway returns the default Docker gateway.
func DefaultGateway() (net.IP, error) {
	fields, err := defaultGatewayFields()
	if err != nil || len(fields) < 3 {
		return nil, err
	}

	ipInt, err := strconv.ParseUint(fields[2], 16, 32)
	if err != nil {
		return nil, fmt.Errorf("unable to parse ip %s from route file: %s", fields[2], err)
	}
	ip := make(net.IP, 4)
	binary.LittleEndian.PutUint32(ip, uint32(ipInt))
	return ip, nil
}

// DefaultHostIPs returns the IP addresses bound to the default network interface.
// The default network interface is the one connected to the network gateway, and it is determined
// by parsing the routing table file in the proc file system.
func DefaultHostIPs() ([]string, error) {
	fields, err := defaultGatewayFields()
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("missing interface information from routing file")
	}
	iface, err := net.InterfaceByName(fields[0])
	if err != nil {
		return nil, err
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}

	ips := make([]string, len(addrs))
	for i, addr := range addrs {
		// Translate CIDR blocks into IPs, if necessary
		ips[i] = strings.Split(addr.String(), "/")[0]
	}

	return ips, nil
}

// defaultGatewayFields extracts the fields associated to the interface connected
// to the network gateway from the linux routing table. As an example, for the given file in /proc/net/routes:
//
// Iface   Destination  Gateway   Flags  RefCnt  Use  Metric  Mask      MTU  Window  IRTT
// enp0s3  00000000     0202000A  0003   0       0    0       00000000  0    0       0
// enp0s3  0002000A     00000000  0001   0       0    0       00FFFFFF  0    0       0
//
// The returned value would be ["enp0s3","00000000","0202000A","0003","0","0","0","00000000","0","0","0"]
//
func defaultGatewayFields() ([]string, error) {
	procRoot := config.Datadog.GetString("proc_root")
	netRouteFile := filepath.Join(procRoot, "net", "route")
	f, err := os.Open(netRouteFile)
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			log.Errorf("Unable to open %s: %s", netRouteFile, err)
			return nil, nil
		}
		// Unknown error types will bubble up for handling.
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 1 && fields[1] == "00000000" {
			return fields, nil
		}
	}

	return nil, fmt.Errorf("couldn't retrieve default gateway information")
}
