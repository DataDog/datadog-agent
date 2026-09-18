// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package receivers

import "testing"

func TestProducerProfileIsIndependentOfArtifactProvider(t *testing.T) {
	for _, provider := range []string{"existing-image", "local-build", "omnibus-package"} {
		t.Run(provider, func(t *testing.T) {
			p := ProducerProfile{RouteContract: RouteContract, Roles: []ProducerRole{CoreAgent, TraceAgent, ProcessAgent}}
			if err := p.Require(CoreAgent, TraceAgent); err != nil {
				t.Fatal(err)
			}
			if err := p.Require(ClusterAgent); err == nil {
				t.Fatal("unattested role accepted")
			}
		})
	}
	for _, p := range []ProducerProfile{{}, {RouteContract: "unknown", Roles: []ProducerRole{CoreAgent}}, {RouteContract: RouteContract, Roles: []ProducerRole{"build-recipe-is-not-a-role"}}} {
		if err := p.Validate(); err == nil {
			t.Fatal("invalid capability profile accepted")
		}
	}
}
