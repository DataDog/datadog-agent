// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/local/kubernetes/kindinfra"
)

// provisionConfig is the resolved provisioner name/options, whichever the
// TestDefinition's YAML `provisioner` section decoded into.
type provisionConfig struct {
	Provisioner string          `json:"provisioner"`
	Options     json.RawMessage `json:"options"`
}

type provisionerFactory func(options json.RawMessage) (provisioners.UntypedProvisioner, error)

var provisionerRegistry = map[string]provisionerFactory{
	"kind": newKindProvisioner,
}

func newKindProvisioner(options json.RawMessage) (provisioners.UntypedProvisioner, error) {
	opts := struct {
		KubeVersion       string `json:"kubeVersion"`
		WithoutFakeIntake bool   `json:"withoutFakeIntake"`
	}{KubeVersion: "1.31"}
	if len(options) > 0 {
		if err := json.Unmarshal(options, &opts); err != nil {
			return nil, fmt.Errorf("parsing kind provisioner options: %w", err)
		}
	}

	kopts := []kindinfra.Option{kindinfra.WithKubeVersion(opts.KubeVersion)}
	if opts.WithoutFakeIntake {
		kopts = append(kopts, kindinfra.WithoutFakeIntake())
	}
	return kindinfra.New(kopts...), nil
}

func resolveProvisioner(cfg provisionConfig) (provisioners.UntypedProvisioner, error) {
	factory, ok := provisionerRegistry[cfg.Provisioner]
	if !ok {
		known := make([]string, 0, len(provisionerRegistry))
		for name := range provisionerRegistry {
			known = append(known, name)
		}
		return nil, fmt.Errorf("unknown provisioner %q, known: %s", cfg.Provisioner, strings.Join(known, ", "))
	}
	return factory(cfg.Options)
}
