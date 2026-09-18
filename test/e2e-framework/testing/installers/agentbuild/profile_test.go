// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentbuild

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
	"strings"
	"testing"
)

func TestBinaryCapabilityEvidenceIsCoreSourceAndInventoryBound(t *testing.T) {
	r := binaryFixture(t)
	if r.RequireBinaryRouting() == nil {
		t.Fatal("unprofiled bundle accepted")
	}
	r.Profile = &receivers.ProducerProfile{RouteContract: receivers.RouteContract, Roles: []receivers.ProducerRole{receivers.CoreAgent}}
	if r.RequireBinaryRouting() == nil {
		t.Fatal("profile without source evidence accepted")
	}
	r.Provenance.SourceSHA256 = strings.Repeat("a", 64)
	r.Provenance.Options = map[string]string{"runtimeLayout": "bazel-embedded-absolute-prefix"}
	if err := r.RequireBinaryRouting(); err != nil {
		t.Fatal(err)
	}
	r.Profile.Roles = append(r.Profile.Roles, receivers.TraceAgent)
	if r.RequireBinaryRouting() == nil {
		t.Fatal("runtime image inferred to run trace-agent")
	}
	r.Profile.Roles = r.Profile.Roles[:1]
	r.Binary.Executable.SHA256 = strings.Repeat("b", 64)
	if r.RequireBinaryRouting() == nil {
		t.Fatal("changed executable accepted under previous profile")
	}
}
