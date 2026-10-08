// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/stretchr/testify/assert"
)

var _ = declareInlineConfig(TestSecurityProfileV2DNSResponse)

func TestSecurityProfileV2DNSResponse(t *testing.T) {
	testSecurityProfileV2DNSResponse(t, []*rules.RuleDefinition{})
}

var _ = declareInlineConfig(TestSecurityProfileV2DNSFullResponse)

// A rule on the response code makes the kernel send NOERROR responses on the full response path
// instead of the short one.
func TestSecurityProfileV2DNSFullResponse(t *testing.T) {
	testSecurityProfileV2DNSResponse(t, []*rules.RuleDefinition{{
		ID:         "test_dns_full_response",
		Expression: `dns.response.code == NOERROR && dns.question.name == "one.one.one.one"`,
	}})
}

func testSecurityProfileV2DNSResponse(t *testing.T, ruleDefs []*rules.RuleDefinition) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
		// TODO: Oracle because we are missing offsets. See dns_test.go
		return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel()
	})

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{
		enableSecurityProfile: true,
		networkIngressEnabled: true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	dockerInstance, err := test.StartADocker()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstance.stop()

	cmd := dockerInstance.Command("nslookup", []string{"one.one.one.one"}, []string{})
	if _, err = cmd.CombinedOutput(); err != nil {
		t.Fatal(err)
	}

	p := test.probe.PlatformProbe.(*probe.EBPFProbe)
	tags, err := p.Resolvers.TagsResolver.ResolveWithErr(containerutils.ContainerID(dockerInstance.containerID))
	if err != nil {
		t.Fatal(err)
	}
	selector := &api.WorkloadSelectorMessage{Name: utils.GetTagValue("image_name", tags), Tag: "*"}

	// the response carries no process context, it has to be attributed to nslookup's request
	assert.Eventually(t, func() bool {
		msg, err := p.GetProfileManager().SaveSecurityProfile(&api.SecurityProfileSaveParams{Selector: selector})
		if err != nil || msg.GetError() != "" {
			return false
		}
		defer os.Remove(msg.GetFile())

		profile, err := DecodeSecurityProfile(msg.GetFile())
		if err != nil {
			return false
		}
		for _, node := range profile.ActivityTree.FindMatchingRootNodes("nslookup") {
			dnsNode, ok := node.DNSNames["one.one.one.one"]
			if !ok {
				continue
			}
			for _, req := range dnsNode.Requests {
				if req.Response != nil && len(req.Response.IPs) > 0 {
					return true
				}
			}
		}
		return false
	}, 10*time.Second, 500*time.Millisecond, "the DNS response should be recorded in nslookup's profile")
}
