// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package traceroute

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	reverseDNSTimeouts = telemetry.NewStatCounterWrapper("traceroute", "reverse_dns_timeouts", []string{}, "Counter measuring the number of traceroute reverse DNS timeouts")
)

var lookupAddrFn = net.DefaultResolver.LookupAddr

// GetReverseDNSForIP returns the reverse DNS for the given IP address as a net.IP.
func GetReverseDNSForIP(destIP net.IP) string {
	if destIP == nil {
		return ""
	}
	return GetHostname(destIP.String())
}

// GetHostname returns the hostname for the given IP address as a string.
func GetHostname(ipAddr string) string {
	currHost := ""
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	currHostList, err := lookupAddrFn(ctx, ipAddr)
	if errors.Is(err, context.Canceled) {
		reverseDNSTimeouts.Inc()
	}

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

func EnrichPathWithRDNS(rdnsQuerier rdnsquerier.Component, timeout time.Duration, path payload.NetworkPath) payload.NetworkPath {
	// collect unique IP addresses from destination and hops
	ipSet := make(map[string]struct{}, len(path.Hops)+1) // +1 for destination
	ipSet[path.Destination.IPAddress] = struct{}{}
	for _, hop := range path.Hops {
		if !hop.Reachable {
			continue
		}
		ipSet[hop.IPAddress] = struct{}{}
	}
	ipAddrs := make([]string, 0, len(ipSet))
	for ip := range ipSet {
		ipAddrs = append(ipAddrs, ip)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// perform reverse DNS lookup on destination and hops
	results := rdnsQuerier.GetHostnames(ctx, ipAddrs)
	// if len(results) != len(ipAddrs) {
	// 	s.statsdClient.Incr(reverseDNSLookupMetricPrefix+"results_length_mismatch", []string{}, 1) //nolint:errcheck
	// 	s.logger.Debugf("Reverse lookup failed for all hops in path from %s to %s", path.Source.Hostname, path.Destination.Hostname)
	// }

	// assign resolved hostnames to destination and hops
	hostname := getReverseDNSResult(path.Destination.IPAddress, results)
	// if hostname is blank, use what's given by traceroute
	// TODO: would it be better to move the logic up from the traceroute command?
	// benefit to the current approach is having consistent behavior for all paths
	// both static and dynamic
	if hostname != "" {
		path.Destination.ReverseDNSHostname = hostname
	}

	for i, hop := range path.Hops {
		if !hop.Reachable {
			continue
		}
		hostname := getReverseDNSResult(hop.IPAddress, results)
		if hostname != "" {
			path.Hops[i].Hostname = hostname
		}
	}

	return path
}

func getReverseDNSResult(ipAddr string, results map[string]rdnsquerier.ReverseDNSResult) string {
	result, ok := results[ipAddr]
	if !ok {
		// s.statsdClient.Incr(reverseDNSLookupFailuresMetricName, []string{"reason:absent"}, 1) //nolint:errcheck
		// s.logger.Tracef("Reverse DNS lookup failed for IP %s", ipAddr)
		return ""
	}
	if result.Err != nil {
		// s.statsdClient.Incr(reverseDNSLookupFailuresMetricName, []string{"reason:error"}, 1) //nolint:errcheck
		// s.logger.Tracef("Reverse lookup failed for hop IP %s: %s", ipAddr, result.Err)
		return ""
	}
	// if result.Hostname == "" {
	// 	s.statsdClient.Incr(reverseDNSLookupSuccessesMetricName, []string{"status:empty"}, 1) //nolint:errcheck
	// } else {
	// 	s.statsdClient.Incr(reverseDNSLookupSuccessesMetricName, []string{"status:found"}, 1) //nolint:errcheck
	// }
	return result.Hostname
}
