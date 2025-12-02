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
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// Purpose of the test:
// If any of the DNS packets fails to be decoded, creates the `failed_dns` event to indicate that it happened
// The cases are:
// - Request
// - Full response
// - Short response
//
// Notice: Not all invalid DNS packets emit an event, as they might get filtered in the kernel side before
// getting to userspace (for example, an inbound packet with the request flag will get filtered before processing)

func getPayloadBytes(customEvent *events.CustomEvent) (string, error) {
	b, err := customEvent.MarshalJSON()
	if err != nil {
		return "", err
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return "", err
	}

	var payloadB64 string
	if err := json.Unmarshal(m["payload"], &payloadB64); err != nil {
		return "", err
	}

	decoded, err := base64.StdEncoding.DecodeString(payloadB64)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(decoded), nil
}

func TestFailedDNSFullResponse(t *testing.T) {
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

	t.Run("failed-dns-full-dns-response", func(t *testing.T) {
		payload := "0000000000000000000000000800450000a41fbd400001115b567f0000357f0000010035d7140090fed7deadb0ef11111111111111111111111111706c6503636f6d0000010001c00c00010001000000ea0004600780c6c00c00010001000000ea000417c0e454c00c00010001000000ea000417d7008ac00c00010001000000ea000417d70088c00c00010001000000ea000417c0e450c00c00010001000000ea0004600780af000029ffd6"
		err = test.GetCustomEventSent(t, func() error {
			err = injectHexDump("lo", payload)
			return nil
		}, func(r *rules.Rule, customEvent *events.CustomEvent) bool {
			if r.Rule.ID == "failed_dns" {
				decodedStr, err := getPayloadBytes(customEvent)
				if err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, decodedStr, payload[len(payload)-len(decodedStr):])
				return true
			}
			return false
		}, 3*time.Second, model.CustomEventType, events.FailedDNSRuleID)

		if err != nil {
			t.Fatal(err)
		}
	})
}
func TestFailedDNSRequest(t *testing.T) {
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

	t.Run("failed-dns-request", func(t *testing.T) {
		payload := "00000000000000000000000008004500003c853c40004011b7727f0000017f000001b0e100350028fe3b7069636b207570207468652070686f6e6500776861616161617a61616161610a"
		err = test.GetCustomEventSent(t, func() error {
			err = injectHexDump("lo", payload)
			return nil
		}, func(r *rules.Rule, customEvent *events.CustomEvent) bool {
			if r.Rule.ID == "failed_dns" {
				decodedStr, err := getPayloadBytes(customEvent)
				if err != nil {
					t.Fatal(err)
				}

				assert.Equal(t, "6970752068776161200070686f6e65", decodedStr)
				return true
			}
			return false
		}, 3*time.Second, model.CustomEventType, events.FailedDNSRuleID)

		if err != nil {
			t.Fatal(err)
		}
	})
}
