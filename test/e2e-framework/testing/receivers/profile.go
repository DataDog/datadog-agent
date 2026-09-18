// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package receivers

import "fmt"

// RouteContract names the configuration surface exercised by the shared
// renderers. Artifact providers attest this contract; tags/recipes are not proof.
const RouteContract = "agent-outbound-v1"

// ProducerRole describes an emitting process, independently of packaging/builds.
type ProducerRole string

const (
	CoreAgent           ProducerRole = "core-agent"
	TraceAgent          ProducerRole = "trace-agent"
	ProcessAgent        ProducerRole = "process-agent"
	ClusterAgent        ProducerRole = "cluster-agent"
	ClusterChecksRunner ProducerRole = "cluster-checks-runner"
)

// ProducerProfile is capability evidence supplied by the artifact owner. An
// existing image, local build or package provider can supply the same profile.
// Receivers never interpret artifact identities, repositories, version tags or
// build recipes. The owner must bind this attestation to the installed bytes.
// It is deliberately not an operator-supplied YAML escape hatch.
type ProducerProfile struct {
	RouteContract string
	Roles         []ProducerRole
}

func (p ProducerProfile) Validate() error {
	if p.RouteContract != RouteContract || len(p.Roles) == 0 {
		return fmt.Errorf("producer profile requires the supported route contract and emitting roles")
	}
	seen := map[ProducerRole]bool{}
	for _, role := range p.Roles {
		switch role {
		case CoreAgent, TraceAgent, ProcessAgent, ClusterAgent, ClusterChecksRunner:
		default:
			return fmt.Errorf("unsupported producer role %q", role)
		}
		if seen[role] {
			return fmt.Errorf("duplicate producer role %q", role)
		}
		seen[role] = true
	}
	return nil
}

// Require checks the roles an installation will run, without enabling them.
func (p ProducerProfile) Require(roles ...ProducerRole) error {
	if err := p.Validate(); err != nil {
		return err
	}
	for _, required := range roles {
		found := false
		for _, role := range p.Roles {
			if role == required {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("producer profile lacks %s routing capability", required)
		}
	}
	return nil
}
