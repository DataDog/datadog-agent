// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

// Package tests holds tests related files
import (
	"github.com/DataDog/datadog-agent/pkg/config/env"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/stretchr/testify/assert"
	"net"
	"net/netip"
	"slices"
	"testing"
	"time"
)

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

	// Makes a DNS query for hosts with known CNAMES and check if they're all resolved correctly
	t.Run("cnames-correctly-processed", func(t *testing.T) {
		hostname := "www.bbc.co.uk"
		ipAddresses, err := net.LookupIP(hostname)
		if err != nil {
			t.Fatal(err)
		}

		assert.GreaterOrEqual(t, len(ipAddresses), 1, "Expected at least one IP address for www.bbc.co.uk")

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
			nip, ok := netip.AddrFromSlice(ip)
			if !ok {
				t.Fatalf("Couldn't get IP address for host %s", ip)
			}

			// This test contains a 1 second backoff, and tries 10 times until it fails
			attempts := 10
			for ; attempts != 0; attempts-- {
				list := p.Resolvers.DNSResolver.HostListFromIP(nip)
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
