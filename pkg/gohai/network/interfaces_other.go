// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build !linux

package network

import "net"

func loadInterfaces(ifaces []net.Interface) ([]networkInterface, error) {
	result := make([]networkInterface, len(ifaces))
	for i, iface := range ifaces {
		r := &realNetworkInterface{iface: iface}
		result[i] = r
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		r.addrs, r.addrsErr = iface.Addrs()
	}
	return result, nil
}
