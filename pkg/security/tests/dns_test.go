// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

package tests

import (
	"fmt"
	"net"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestDNS(t *testing.T) {
	checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
		// TODO: Oracle because we are missing offsets
		return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel()
	})

	if testEnvironment != DockerEnvironment && !config.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_dns",
			Expression: fmt.Sprintf(`dns.question.type == A && dns.question.name == "google.com" && process.file.name == "%s"`, path.Base(executable)),
		},
		{
			ID:         "test_long_query",
			Expression: `dns.question.type == A && dns.question.name.length > 60 && process.file.name == "testsuite"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("dns", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			_, err = net.LookupIP("google.com")
			if err != nil {
				return err
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_dns")
			assert.Equal(t, "google.com", event.DNS.Name, "wrong domain name")

			test.validateDNSSchema(t, event)
		})
	})

	t.Run("dns-case", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			_, err = net.LookupIP("GOOGLE.COM")
			if err != nil {
				return err
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_dns")
			assert.Equal(t, "GOOGLE.COM", event.DNS.Name, "wrong domain name")

			test.validateDNSSchema(t, event)
		})
	})

	t.Run("dns-long-domain", func(t *testing.T) {
		longDomain := strings.Repeat("A", 58) + ".COM"
		test.WaitSignal(t, func() error {
			net.LookupIP(longDomain)
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_long_query")
			assert.Equal(t, "dns", event.GetType(), "wrong event type")
			assert.Equal(t, longDomain, event.DNS.Name, "wrong domain name")

			test.validateDNSSchema(t, event)
		})
	})
}
