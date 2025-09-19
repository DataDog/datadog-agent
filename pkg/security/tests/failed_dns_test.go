// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

// Package tests holds tests related files
import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestFailedDNS(t *testing.T) {
	SkipIfNotAvailable(t)
	checkNetworkCompatibility(t)

	ruleDefs := []*rules.RuleDefinition{{
		ID:         "failed_dns_rule",
		Expression: `dns.response.code != NXDOMAIN`,
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

	payload := "0000000000000000000000000800450000a41fbd400001115b567f0000357f0000010035d7140090fed7deadb0ef11111111111111111111111111706c6503636f6d0000010001c00c00010001000000ea0004600780c6c00c00010001000000ea000417c0e454c00c00010001000000ea000417d7008ac00c00010001000000ea000417d70088c00c00010001000000ea000417c0e450c00c00010001000000ea0004600780af000029ffd6000000000000"
	err = test.GetCustomEventSent(t, func() error {
		err = injectHexDump("lo", payload)
		return nil
	}, func(r *rules.Rule, customEvent *events.CustomEvent) bool {
		if r.Rule.ID == "failed_dns" {
			b, _ := customEvent.MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}

			var m map[string]json.RawMessage
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatal(err)
			}

			var payloadB64 string
			if err := json.Unmarshal(m["payload"], &payloadB64); err != nil {
				t.Fatal(err)
			}

			decoded, err := base64.StdEncoding.DecodeString(payloadB64)
			if err != nil {
				t.Fatal(err)
			}
			decodedStr := fmt.Sprintf("%x", decoded)

			assert.Equal(t, decodedStr, payload[len(payload)-len(decodedStr):])
			return true
		}
		return false
	}, 3*time.Second, model.CustomEventType, events.FailedDNSRuleID)

	if err != nil {
		t.Fatal(err)
	}
}
