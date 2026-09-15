// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

// Package tests holds tests related files
import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
)

const DNSPort = 5553

const (
	dnsTransactionIDOffset    = 14 + 20 + 8 // Ethernet + IPv4 + UDP headers
	dnsResponseInjectInterval = 500 * time.Millisecond
)

// waitForDNSResponseRule injects a DNS response on the loopback interface and waits for
// ruleID to fire, re-injecting on a ticker (with a fresh transaction id each time) until it
// matches. Under heavy event load a single DNS response can be dropped by the perf ring
// buffer; because the wait injects only once, that dropped event makes the test flake.
// Retrying survives a drop, and bumping the transaction id dodges the kernel's (id, size)
// 1s de-duplication that would otherwise silently discard identical retries.
func (tm *testModule) waitForDNSResponseRule(tb testing.TB, hexDump, ruleID string, cb onRuleHandler) {
	tb.Helper()

	packet, err := hexToPacket(hexDump)
	if err != nil {
		tb.Fatal(err)
	}

	stop := make(chan struct{})
	var once sync.Once
	stopInjecting := func() { once.Do(func() { close(stop) }) }
	tb.Cleanup(stopInjecting)

	tm.WaitSignalFromRule(tb, func() error {
		go func() {
			id := binary.BigEndian.Uint16(packet[dnsTransactionIDOffset:])
			ticker := time.NewTicker(dnsResponseInjectInterval)
			defer ticker.Stop()
			for {
				binary.BigEndian.PutUint16(packet[dnsTransactionIDOffset:], id)
				_ = sendRawPacket("lo", packet)
				id++
				select {
				case <-stop:
					return
				case <-ticker.C:
				}
			}
		}()
		return nil
	}, func(event *model.Event, rule *rules.Rule) {
		stopInjecting()
		cb(event, rule)
	}, ruleID)
}

// We need to bind to an address in order to tell that the netflow is related to this IP address so that
// the process context can be resolved correctly
func justBind() *net.UDPConn {
	addr := ":" + strconv.Itoa(DNSPort)

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		fmt.Println("Error resolving UDP address:", err)
		os.Exit(1)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		fmt.Println("Error creating UDP socket:", err)
		os.Exit(1)
	}

	return conn
}

var _ = declare(TestDNSResponse, testOpts{
	dnsPort: DNSPort,
})

func TestDNSResponse(t *testing.T) {
	SkipIfNotAvailable(t)
	checkNetworkCompatibility(t)

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	ruleDefsRcodeOK := []*rules.RuleDefinition{
		{
			ID:         "dns_response_ok",
			Expression: `dns.response.code == NOERROR && dns.question.name == "www.datadoghq.eu" && dns.response.ips in [34.149.115.158]`,
		},
	}

	ruleDefsRcodeNXDomain := []*rules.RuleDefinition{
		{
			ID:         "dns_response_nok",
			Expression: `dns.response.code == NXDOMAIN && dns.question.name == "www.datadawg.eu"`,
		},
	}

	ruleDefsResponseIPOnly := []*rules.RuleDefinition{
		{
			ID:         "dns_response_ip_only",
			Expression: `dns.response.ips in [34.149.115.158]`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefsRcodeOK)
	if err != nil {
		t.Fatal(err)
	}

	defer justBind().Close()

	t.Run("catch-dns-rcode-zero", func(t *testing.T) {
		hexDump := "00000000000000000000000008004500004ef53c40000111862c7f0000357f00000115b18bb0003a96af5ac281800001000100000000037777770964617461646f6768710265750000010001c00c000100010000003c00042295739e"
		test.waitForDNSResponseRule(t, hexDump, "dns_response_ok", func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "dns_response_ok")
			assert.Equal(t, "dns", event.GetType(), "wrong event type")
			assert.Equal(t, "www.datadoghq.eu", event.DNS.Question.Name, "wrong domain name")
			assert.Equal(t, uint8(model.DNSResponseCodeConstants["NOERROR"]), event.DNS.Response.ResponseCode, "wrong response code")
			assert.Len(t, event.DNS.Response.IPs, 1, "wrong resolved IP count")
			assert.Equal(t, "34.149.115.158", event.DNS.Response.IPs[0].IP.String(), "wrong resolved IP")
			assert.Empty(t, event.DNS.Response.CNames, "unexpected CNAMEs")

			test.validateDNSSchema(t, event)
		})
	})
	test.Close()

	test, err = newTestModule(t, nil, ruleDefsResponseIPOnly)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("catch-dns-response-ip-only", func(t *testing.T) {
		hexDump := "00000000000000000000000008004500004ef53c40000111862c7f0000357f00000115b18bb0003a96ae5ac381800001000100000000037777770964617461646f6768710265750000010001c00c000100010000003c00042295739e"
		test.waitForDNSResponseRule(t, hexDump, "dns_response_ip_only", func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "dns_response_ip_only")
			assert.Equal(t, "dns", event.GetType(), "wrong event type")
			assert.Equal(t, "www.datadoghq.eu", event.DNS.Question.Name, "wrong domain name")
			assert.Len(t, event.DNS.Response.IPs, 1, "wrong resolved IP count")
			assert.Equal(t, "34.149.115.158", event.DNS.Response.IPs[0].IP.String(), "wrong resolved IP")

			test.validateDNSSchema(t, event)
		})
	})
	test.Close()

	test, err = newTestModule(t, nil, ruleDefsRcodeNXDomain)

	if err != nil {
		t.Fatal(err)
	}
	t.Run("catch-dns-rcode-nxdomain", func(t *testing.T) {
		hexDump := "0000000000000000000000000800450000732e9d400001114ca77f0000357f00000115b1d778005fba5b2a7281830001000000010000037777770864617461646177670265750000010001c0190006000100000258002a02736903646e73c0190474656368056575726964c019423b7e6500000e10000007080036ee8000000258"
		test.waitForDNSResponseRule(t, hexDump, "dns_response_nok", func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "dns_response_nok")
			assert.Equal(t, "dns", event.GetType(), "wrong event type")
			assert.Equal(t, "www.datadawg.eu", event.DNS.Question.Name, "wrong domain name")
			assert.Equal(t, uint8(model.DNSResponseCodeConstants["NXDOMAIN"]), event.DNS.Response.ResponseCode, "wrong response code")
			test.validateDNSSchema(t, event)
		})
	})
	test.Close()
}

var _ = declare(TestDNSResponseDiscarder, testOpts{
	dnsPort: DNSPort,
})

func TestDNSResponseDiscarder(t *testing.T) {
	SkipIfNotAvailable(t)
	checkNetworkCompatibility(t)
	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	ruleDefsRcodeOK := []*rules.RuleDefinition{
		{
			ID:         "dns_response_ok",
			Expression: `dns.response.code == NXDOMAIN`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefsRcodeOK)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	defer justBind().Close()

	t.Run("noerror-packet-is-discarded", func(_ *testing.T) {
		err = test.GetProbeEvent(func() error {
			// Send packet with rcode NOERROR, but the rule expects NODOMAIN, therefore it should be discarded
			hexDump := "00000000000000000000000008004500004ef53c40000111862c7f0000357f00000115b18bb0003a96af5ac281800001000100000000037777770964617461646f6768710265750000010001c00c000100010000003c00042295739e"
			err = injectHexDump("lo", hexDump)
			return nil
		}, func(event *model.Event) bool {
			return event.DNS.Question.Name == "www.datadoghq.eu"
		}, 3*time.Second, model.DNSEventType)
	})

	// Packet should never be received
	assert.NotEqual(t, err, nil)
}
