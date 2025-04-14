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
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
	"net"
	"os"
	"testing"
	"time"
)

func justBind() *net.UDPConn {
	addr := ":5553"

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
			Expression: `dns_response.response_code == 0 && dns_response.question.name == "www.datadoghq.eu"`,
		},
	}

	ruleDefsRcodeNXDomain := []*rules.RuleDefinition{
		{
			ID:         "dns_response_nok",
			Expression: `dns_response.response_code == 3 && dns_response.question.name == "www.datadawg.eu"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefsRcodeOK, withStaticOpts(testOpts{
		dnsPort: 5553,
	}))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("catch-dns-rcode-zero", func(t *testing.T) {
		c := justBind()

		test.WaitSignal(t, func() error {
			hexDump := "00000000000000000000000008004500004ef53c40000111862c7f0000357f00000115b18bb0003a96af5ac281800001000100000000037777770964617461646f6768710265750000010001c00c000100010000003c00042295739e"

			time.Sleep(1 * time.Second)
			err = injectHexDump("lo", hexDump)

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			fmt.Println("Event received by the test", event.DNSResponse)
			assertTriggeredRule(t, rule, "dns_response_ok")
			assert.Equal(t, "dns_response", event.GetType(), "wrong event type")
			assert.Equal(t, "www.datadoghq.eu", event.DNSResponse.Question.Name, "wrong domain name")
			//test.validateDNSSchema(t, event)
		})

		c.Close()
	})
	test.Close()

	test, err = newTestModule(t, nil, ruleDefsRcodeNXDomain, withStaticOpts(testOpts{networkIngressEnabled: true}))
	if err != nil {
		t.Fatal(err)
	}
	t.Run("catch-dns-rcode-nxdomain", func(t *testing.T) {
		c := justBind()
		test.WaitSignal(t, func() error {
			hexDump := "0000000000000000000000000800450000732e9d400001114ca77f0000357f00000115b1d778005fba5b2a7281830001000000010000037777770864617461646177670265750000010001c0190006000100000258002a02736903646e73c0190474656368056575726964c019423b7e6500000e10000007080036ee8000000258"
			err = injectHexDump("lo", hexDump)
			if err != nil {
				t.Error(err)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "dns_response_nok")
			assert.Equal(t, "dns_response", event.GetType(), "wrong event type")
			assert.Equal(t, "www.datadawg.eu", event.DNSResponse.Question.Name, "wrong domain name")
			//test.validateDNSSchema(t, event)
		})
		c.Close()
	})
	test.Close()

}
