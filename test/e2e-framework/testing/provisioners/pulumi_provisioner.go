// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provisioners

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/infra"
)

const (
	pulumiProvisionerDefaultID = "pulumi"
)

// PulumiEnvRunFunc is a function that runs a Pulumi program with a given environment.
type PulumiEnvRunFunc[Env any] func(ctx *pulumi.Context, env *Env) error

// PulumiProvisioner is a provisioner based on Pulumi with binding to an environment.
type PulumiProvisioner[Env any] struct {
	id           string
	runFunc      PulumiEnvRunFunc[Env]
	configMap    runner.ConfigMap
	diagnoseFunc func(ctx context.Context, stackName string) (string, error)
}

var (
	_ TypedProvisioner[any] = &PulumiProvisioner[any]{}
	_ UntypedProvisioner    = &PulumiProvisioner[any]{}
)

// NewTypedPulumiProvisioner returns a new PulumiProvisioner.
func NewTypedPulumiProvisioner[Env any](id string, runFunc PulumiEnvRunFunc[Env], configMap runner.ConfigMap) *PulumiProvisioner[Env] {
	if id == "" {
		id = pulumiProvisionerDefaultID
	}

	return &PulumiProvisioner[Env]{
		id:        id,
		runFunc:   runFunc,
		configMap: configMap,
	}
}

// NewUntypedPulumiProvisioner returns a new PulumiProvisioner without env binding.
func NewUntypedPulumiProvisioner(id string, runFunc pulumi.RunFunc, configMap runner.ConfigMap) *PulumiProvisioner[any] {
	return NewTypedPulumiProvisioner(id, func(ctx *pulumi.Context, _ *any) error {
		return runFunc(ctx)
	}, configMap)
}

// ID returns the ID of the provisioner.
func (pp *PulumiProvisioner[Env]) ID() string {
	return pp.id
}

// Provision runs the Pulumi program and returns the raw resources.
func (pp *PulumiProvisioner[Env]) Provision(ctx context.Context, stackName string, logger io.Writer) (RawResources, error) {
	return pp.ProvisionEnv(ctx, stackName, logger, nil)
}

// ProvisionEnv runs the Pulumi program with a given environment and returns the raw resources.
func (pp *PulumiProvisioner[Env]) ProvisionEnv(ctx context.Context, stackName string, logger io.Writer, env *Env) (RawResources, error) {
	_, stackOutput, err := infra.GetStackManager().GetStackNoDeleteOnFailure(
		ctx,
		stackName,
		func(ctx *pulumi.Context) error {
			return pp.runFunc(ctx, env)
		},
		infra.WithConfigMap(pp.configMap),
		infra.WithLogWriter(logger),
	)

	if err != nil {
		return nil, err
	}

	resources := make(RawResources, len(stackOutput.Outputs))
	secretKeys := make(map[string]bool, len(stackOutput.Outputs))
	for key, value := range stackOutput.Outputs {
		// Skipping legacy outputs that are not maps
		if reflect.TypeOf(value.Value).Kind() != reflect.Map {
			continue
		}

		// Unfortunately we don't have access to Pulumi raw data
		marshalled, err := json.MarshalIndent(value.Value, "", "\t")
		if err != nil {
			return nil, fmt.Errorf("unable to marshal output key: %s, err: %w", key, err)
		}

		resources[key] = marshalled
		secretKeys[key] = value.Secret
	}

	_, err = logger.Write([]byte(fmt.Sprintf("Pulumi stack %s successfully provisioned\nResources:\n%v\n\n", stackName, dumpRawResources(resources, secretKeys))))
	if err != nil {
		// Log the error but don't fail the provisioning
		fmt.Printf("Failed to write log: %v\n", err)
	}

	return resources, nil
}

// dumpRawResources renders resources for logging. For any key marked secret in secretKeys,
// only the "password" field is redacted so Pulumi secrets (e.g. the Windows admin password)
// never appear in plain text in CI/test logs, while the rest of the resource (address,
// username, port, ...) stays visible. The real values are still returned via RawResources
// for in-process use.
func dumpRawResources(resources RawResources, secretKeys map[string]bool) string {
	var builder strings.Builder
	for key, value := range resources {
		if secretKeys[key] {
			fmt.Fprintf(&builder, "%s: %s\n", key, redactPassword(value))
			continue
		}
		fmt.Fprintf(&builder, "%s: %s\n", key, value)
	}
	return builder.String()
}

// redactPassword replaces every "password" field found anywhere in a marshalled resource,
// including inside nested objects/arrays (e.g. a HostAgent output embedding its Host), with
// a placeholder, leaving the rest of the JSON untouched. If the top-level value isn't a JSON
// object, the whole value is redacted since we can't isolate the password field safely.
func redactPassword(raw []byte) []byte {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return []byte("[secret value redacted from logs]")
	}
	return redactPasswordDeep(raw)
}

// redactPasswordDeep walks a JSON value, replacing any "password" object field with a
// placeholder at every nesting level. Values that aren't objects/arrays (or that fail to
// re-marshal) are returned unchanged.
func redactPasswordDeep(raw json.RawMessage) json.RawMessage {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil {
		for k, v := range obj {
			if k == "password" {
				obj[k] = json.RawMessage(`"[redacted]"`)
				continue
			}
			obj[k] = redactPasswordDeep(v)
		}
		marshalled, err := json.MarshalIndent(obj, "", "\t")
		if err != nil {
			return raw
		}
		return marshalled
	}

	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		for i, v := range arr {
			arr[i] = redactPasswordDeep(v)
		}
		marshalled, err := json.MarshalIndent(arr, "", "\t")
		if err != nil {
			return raw
		}
		return marshalled
	}

	return raw
}

// Diagnose runs the diagnose function if it is set diagnoseFunc
func (pp *PulumiProvisioner[Env]) Diagnose(ctx context.Context, stackName string) (string, error) {
	if pp.diagnoseFunc != nil {
		return pp.diagnoseFunc(ctx, stackName)
	}
	return "\n\n--- No Diagnose function set ---\n\n", nil
}

// SetDiagnoseFunc sets the diagnose function.
func (pp *PulumiProvisioner[Env]) SetDiagnoseFunc(diagnoseFunc func(ctx context.Context, stackName string) (string, error)) {
	pp.diagnoseFunc = diagnoseFunc
}

// Destroy deletes the Pulumi stack.
func (pp *PulumiProvisioner[Env]) Destroy(ctx context.Context, stackName string, logger io.Writer) error {
	return infra.GetStackManager().DeleteStack(ctx, stackName, logger)
}
