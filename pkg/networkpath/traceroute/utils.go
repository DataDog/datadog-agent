// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package traceroute

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var lookupAddrFn = net.DefaultResolver.LookupAddr

// getDestinationHostname tries to convert the input destinationHost to hostname.
// When input destinationHost is an IP, a reverse DNS call is made to convert it into a hostname.
func getDestinationHostname(destinationHost string) string {
	destIP := net.ParseIP(destinationHost)
	if destIP != nil {
		reverseDNSHostname := getHostname(destinationHost)
		if reverseDNSHostname != "" {
			return reverseDNSHostname
		}
	}
	return destinationHost
}

func getHostname(ipAddr string) string {
	// TODO: this reverse lookup appears to have some standard timeout that is relatively
	// high. Consider switching to something where there is greater control.
	// Possible solution is to use https://pkg.go.dev/net#Resolver.LookupAddr to specify a context with a timeout.
	currHost := ""
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	currHostList, _ := lookupAddrFn(ctx, ipAddr)
	log.Debugf("Reverse DNS List: %+v", currHostList)

	if len(currHostList) > 0 {
		// TODO: Reverse DNS: Do we need to handle cases with multiple DNS being returned?
		currHost = currHostList[0]
	} else {
		currHost = ipAddr
	}
	// Trim trailing `.` in hostname since users are more familiar with this form.
	currHost = strings.TrimRight(currHost, ".")
	return currHost
}
