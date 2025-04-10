// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

// Package tests holds tests related files
import (
	"fmt"
	"net"
	"net/netip"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/stretchr/testify/assert"
)

func injectHexDump(iface string, hexDump string) error {
	packetData := []byte{}
	for i := 0; i < len(hexDump); i += 2 {
		var byteValue byte
		_, err := fmt.Sscanf(hexDump[i:i+2], "%x", &byteValue)
		if err != nil {
			return fmt.Errorf("error converting hex dump to bytes: %v", err)
		}
		packetData = append(packetData, byteValue)
	}

	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, syscall.ETH_P_IP)
	if err != nil {
		return fmt.Errorf("failed to create raw socket: %v", err)
	}
	defer syscall.Close(fd)

	ifaceIndex, err := getInterfaceIndex(iface)
	if err != nil {
		return fmt.Errorf("failed to get interface index: %v", err)
	}

	addr := &syscall.SockaddrLinklayer{
		Protocol: syscall.ETH_P_IP,
		Ifindex:  ifaceIndex,
	}

	err = syscall.Sendto(fd, packetData, 0, addr)
	if err != nil {
		return fmt.Errorf("failed to send packet: %v", err)
	}

	return nil
}

func getInterfaceIndex(iface string) (int, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return 0, err
	}

	for _, i := range interfaces {
		if i.Name == iface {
			return i.Index, nil
		}
	}

	return 0, fmt.Errorf("interface %s not found", iface)
}

func TestDNSResolver(t *testing.T) {
	SkipIfNotAvailable(t)
	checkNetworkCompatibility(t)

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	test, err := newTestModule(t, nil, nil, withStaticOpts(testOpts{networkIngressEnabled: true}))
	if err != nil {
		t.Fatal(err)
	}

	defer test.Close()
	p, _ := test.probe.PlatformProbe.(*sprobe.EBPFProbe)

	// Makes a DNS query for a couple hostnames on a list and checks
	// if the resolver saved all of them on the cache
	t.Run("saves-hostname-for-all-ips", func(t *testing.T) {
		// This test contains a 1 second backoff, and tries 10 times until it fails
		attempts := 10

		savedAllHostnamesFor := func(hostname string, ipAddresses []net.IP) {
			assert.GreaterOrEqual(t, len(ipAddresses), 1)

			for _, ipAddress := range ipAddresses {
				for ; attempts != 0; attempts-- {
					nip, ok := netip.AddrFromSlice(ipAddress)
					if !ok {
						t.Fatal("Couldn't get an IP address. Network issues?")
					}
					list := p.Resolvers.DNSResolver.HostListFromIP(nip)

					if len(list) != 0 {
						assert.True(t, slices.Contains(list, hostname))
						break
					}

					time.Sleep(1 * time.Second)
				}
			}
		}

		hostList := []string{"perdu.com", "datadoghq.com", "datadoghq.eu", "example.com", "example.org", "example.net"}
		var addresses = make(map[string][]net.IP)

		for _, host := range hostList {
			ipAddresses, err := net.LookupIP(host)
			if err != nil {
				t.Fatalf("couldn't get IP address for host %s", host)
			}

			addresses[host] = ipAddresses
		}

		for hostname, addresses := range addresses {
			savedAllHostnamesFor(hostname, addresses)
			if attempts == 0 {
				break
			}
		}

		assert.Greater(t, attempts, 0)
	})

	// Injects a packet with a CNAME response and check if they were processed correctly
	t.Run("cnames-correctly-processed", func(t *testing.T) {
		hostname := "www.bbc.co.uk"

		hexDump := "0000000000000000000000000800450000c633044000011147ed7f0000357f0000010035d41400b2fef9b88381800001000600000001037777770362626302636f02756b0000010001c00c0005000100003b030014037777770362626302636f02756b03707269c010c02b000500010000012c001403626263036d617006666173746c79036e657400c04b0001000100000023000497654051c04b0001000100000023000497658051c04b0001000100000023000497650051c04b000100010000002300049765c051000029ffd6000000000000"

		ipAddresses := []netip.Addr{
			netip.MustParseAddr("151.101.64.81"),
			netip.MustParseAddr("151.101.0.81"),
			netip.MustParseAddr("151.101.192.81"),
			netip.MustParseAddr("151.101.128.81"),
		}

		err = injectHexDump("lo", hexDump)

		expectedCNAMEs := []string{
			"www.bbc.co.uk.pri.bbc.co.uk",
			"bbc.map.fastly.net",
		}

		allCnamesResolved := func(list []string) bool {
			for _, cname := range expectedCNAMEs {
				if !slices.Contains(list, cname) {
					return false
				}
			}
			return true
		}

		for _, ip := range ipAddresses {
			// This test contains a 1 second backoff, and tries 10 times until it fails
			attempts := 10
			for ; attempts != 0; attempts-- {
				list := p.Resolvers.DNSResolver.HostListFromIP(ip)
				if slices.Contains(list, hostname) && allCnamesResolved(list) {
					break
				}
				time.Sleep(1 * time.Second)
			}

			if attempts == 0 {
				t.Fatal("Number of attempts exceeded")
			}
		}
	})
}
