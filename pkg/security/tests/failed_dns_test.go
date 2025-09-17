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
	"testing"
)

func TestFailedDNS(t *testing.T) {
	SkipIfNotAvailable(t)
	checkNetworkCompatibility(t)

	ruleDefs := []*rules.RuleDefinition{{
		ID:         "failed_dns_rule",
		Expression: fmt.Sprintf(`failed_dns.payload == failed_dns.payload`),
	}}

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{networkIngressEnabled: true}))
	if err != nil {
		t.Fatal(err)
	}

	defer test.Close()

	// Injects garbage DNS responses and check the expected event was emitted
	t.Run("failed_dns_response", func(t *testing.T) {
		payload := "DEADBEEF"
		test.WaitSignal(t, func() error {
			fmt.Println("Injecting payload")
			err = injectHexDump("lo", payload)
			return nil
		}, func(event *model.Event, rule *rules.Rule) {

			fmt.Println("Received_injection")
			assertTriggeredRule(t, rule, "failed_dns_rule")

			//test.validateAcceptSchema(t, event)
		})
	})
}
