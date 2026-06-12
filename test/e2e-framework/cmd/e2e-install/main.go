// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// e2e-install installs the Datadog Agent on a pre-provisioned E2E environment
// described by a JSON descriptor file. It is the stand-alone entry point for
// the provision-then-install workflow: after `pulumi up` completes and emits
// an environment descriptor, run e2e-install to deploy the agent without
// re-running Pulumi.
//
// Usage:
//
//	e2e-install --env env.json --spec spec.json
//
// Both files are written by the QA tasks / CI jobs after provisioning.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/cliutil"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/envdesc"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/dockeragent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/helmagent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/hostagent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installspec"
)

func main() {
	envFile := flag.String("env", "", "path to environment descriptor JSON (required)")
	specFile := flag.String("spec", "", "path to install spec JSON (required)")
	flag.Parse()

	if *envFile == "" || *specFile == "" {
		fmt.Fprintln(os.Stderr, "usage: e2e-install --env <env.json> --spec <spec.json>")
		os.Exit(2)
	}

	ctx := cliutil.NewContext()
	defer ctx.RunCleanup()

	desc, err := envdesc.ReadFromFile(*envFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FATAL] cannot read env descriptor: %v\n", err)
		os.Exit(1)
	}

	spec, err := installspec.ReadFromFile(*specFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FATAL] cannot read install spec: %v\n", err)
		os.Exit(1)
	}

	if err := install(ctx, desc, spec); err != nil {
		fmt.Fprintf(os.Stderr, "[FATAL] installation failed: %v\n", err)
		os.Exit(1)
	}

	ctx.Logf("e2e-install: done")
}

func install(ctx *cliutil.CLIContext, desc *envdesc.Descriptor, spec *installspec.Spec) error {
	cloud := installspec.CloudFromString(spec.Cloud)

	switch desc.EnvType {
	case "host":
		env, err := envdesc.LoadEnv[environments.Host](desc, ctx)
		if err != nil {
			return fmt.Errorf("loading host env: %w", err)
		}
		hostagent.Install(ctx, env, spec.Host.HostAgentOptions()...)

	case "windowshost":
		env, err := envdesc.LoadEnv[environments.WindowsHost](desc, ctx)
		if err != nil {
			return fmt.Errorf("loading windows host env: %w", err)
		}
		hostagent.InstallOnWindowsHost(ctx, env, spec.Host.HostAgentOptions()...)

	case "kubernetes":
		env, err := envdesc.LoadEnv[environments.Kubernetes](desc, ctx)
		if err != nil {
			return fmt.Errorf("loading kubernetes env: %w", err)
		}
		if spec.UseOperator {
			return fmt.Errorf("operator install not yet implemented in e2e-install; use UseOperator: false")
		}
		helmagent.Install(ctx, env, cloud, spec.Kubernetes.KubernetesAgentOptions()...)

	case "dockerhost":
		env, err := envdesc.LoadEnv[environments.DockerHost](desc, ctx)
		if err != nil {
			return fmt.Errorf("loading dockerhost env: %w", err)
		}
		dockeragent.Install(ctx, env, spec.Docker.DockerAgentOptions()...)

	default:
		return fmt.Errorf("unsupported env_type %q (supported: host, windowshost, kubernetes, dockerhost)", desc.EnvType)
	}

	return nil
}
