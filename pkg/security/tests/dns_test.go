// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"net"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestDNS(t *testing.T) {
	SkipIfNotAvailable(t)

	checkNetworkCompatibility(t)

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
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
			ID:         "test_rule_dns_lowercase",
			Expression: fmt.Sprintf(`dns.question.type == A && dns.question.name == "perdu.com" && process.file.name == "%s" && process.netns == network.device.netns`, path.Base(executable)),
		},
		{
			ID:         "test_rule_dns_uppercase",
			Expression: fmt.Sprintf(`dns.question.type == A && dns.question.name == "MICROSOFT.COM" && process.file.name == "%s" && process.netns == network.device.netns`, path.Base(executable)),
		},
		{
			ID:         "test_rule_long_query",
			Expression: fmt.Sprintf(`dns.question.type == A && dns.question.name.length > 60 && process.file.name == "%s" && process.netns == network.device.netns`, path.Base(executable)),
		},
		{
			ID:         "test_rule_dns_root_domain",
			Expression: fmt.Sprintf(`dns.question.type == A && dns.question.name.root_domain == "yahoo.com" && process.file.name == "%s" && process.netns == network.device.netns && dns.question.name.root_domain != ${root_domain}`, path.Base(executable)),
			Actions: []*rules.ActionDefinition{
				{
					Set: &rules.SetDefinition{
						Name:         "root_domain",
						Expression:   `dns.question.name.root_domain`,
						DefaultValue: "",
					},
				},
			},
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("dns", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			_, err = net.LookupIP("perdu.com")
			if err != nil {
				return err
			}
			return nil
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "perdu.com", event.DNS.Question.Name, "wrong domain name")

			test.validateDNSSchema(t, event)
		}, "test_rule_dns_lowercase")
	})

	t.Run("dns-case", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			_, err = net.LookupIP("MICROSOFT.COM")
			if err != nil {
				return err
			}
			return nil
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "MICROSOFT.COM", event.DNS.Question.Name, "wrong domain name")

			test.validateDNSSchema(t, event)
		}, "test_rule_dns_uppercase")
	})

	t.Run("dns-root-domain", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			_, err = net.LookupIP("www.yahoo.com")
			if err != nil {
				return err
			}
			return nil
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "www.yahoo.com", event.DNS.Question.Name, "wrong domain name")

			test.validateDNSSchema(t, event)
		}, "test_rule_dns_root_domain")

		// shouldn't trigger anymore because of variable
		err = test.GetEventSent(t, func() error {
			_, err = net.LookupIP("news.yahoo.com")
			if err != nil {
				return err
			}
			return nil
		}, func(_ *rules.Rule, ev *model.Event) bool {
			return ev.DNS.Question.Name == "news.yahoo.com"
		}, time.Second*3, "test_rule_dns_root_domain")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("dns-long-domain", func(t *testing.T) {
		longDomain := strings.Repeat("A", 58) + ".COM"
		test.WaitSignalFromRule(t, func() error {
			net.LookupIP(longDomain)
			return nil
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "dns", event.GetType(), "wrong event type")
			assert.Equal(t, longDomain, event.DNS.Question.Name, "wrong domain name")

			test.validateDNSSchema(t, event)
		}, "test_rule_long_query")
	})
}
