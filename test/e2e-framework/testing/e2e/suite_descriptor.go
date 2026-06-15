// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/envdesc"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

// loadEnvFromDescriptor populates bs.env from a pre-provisioned environment
// descriptor JSON file. It replaces the Pulumi provision step in attach mode.
// After loading, bs.currentProvisioners is set to the original provisioners so
// that PostProvision (agent install) runs correctly.
func (bs *BaseSuite[Env]) loadEnvFromDescriptor(path string) error {
	d, err := envdesc.ReadFromFile(path)
	if err != nil {
		return fmt.Errorf("reading descriptor: %w", err)
	}

	env, err := envdesc.LoadEnv[Env](d, bs)
	if err != nil {
		return fmt.Errorf("deserializing env: %w", err)
	}

	bs.env = env
	// Keep the provisioners so PostProvision (agent install) runs.
	bs.currentProvisioners = provisioners.CopyProvisioners(bs.originalProvisioners)
	return nil
}

// dumpEnvDescriptor writes the current env to a descriptor JSON file at path.
// Called after a successful Pulumi provision so the descriptor can be consumed
// by a subsequent install+test job.
func (bs *BaseSuite[Env]) dumpEnvDescriptor(path string) error {
	if bs.env == nil {
		return fmt.Errorf("env is nil, cannot dump descriptor")
	}
	scenario := ""
	// Extract scenario name from provisioners if available.
	for _, p := range bs.currentProvisioners {
		scenario = p.ID()
		break
	}
	return envdesc.WriteEnvToFile(scenario, envTypeString[Env](), bs.env, path)
}

// envTypeString returns a lowercase env type string for the given Env type.
// This mirrors the envdesc.Descriptor.EnvType convention.
func envTypeString[Env any]() string {
	var zero Env
	t := reflect.TypeOf(&zero).Elem()
	switch t.Name() {
	case "Host":
		return "host"
	case "WindowsHost":
		return "windowshost"
	case "Kubernetes":
		return "kubernetes"
	case "DockerHost":
		return "dockerhost"
	case "ECS":
		return "ecs"
	default:
		return t.Name()
	}
}

// Ensure BaseSuite satisfies common.Context (compile-time check).
var _ common.Context = (*BaseSuite[any])(nil)
