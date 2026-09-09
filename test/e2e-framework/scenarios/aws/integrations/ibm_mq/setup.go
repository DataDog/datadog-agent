// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ibm_mq

import (
	_ "embed"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Embedded provisioning scripts. Each is run as root via `sudo bash -s` with the
// script fed on stdin (see runRootScript): the shebang is an inert first comment
// line, and scenario parameters are prepended as MQ_* assignment lines so they
// reach the root shell regardless of sudo's env_reset policy. Phase-1 skeleton:
// these scripts move into components/integration/ibm_mq during synthesis.
//
//go:embed scripts/mq_setup.sh
var mqSetupScript string

//go:embed scripts/mq_load.sh
var mqLoadScript string

//go:embed scripts/mq_readiness.sh
var mqReadinessScript string

// runRootScript runs the given script as root by feeding it to `sudo bash -s` on
// stdin. See the lustre component for the rationale (a bare Sudo:true Create with a
// shebang runs unprivileged because `sudo #!/bin/bash` comments out the command).
func runRootScript(e config.Env, host *remote.Host, name string, stdin string, opts ...pulumi.ResourceOption) (command.Command, error) {
	return host.OS.Runner().Command(
		e.CommonNamer().ResourceName(name),
		&command.Args{
			Create: pulumi.String("bash -s"),
			Sudo:   true,
			Stdin:  pulumi.String(stdin),
		},
		opts...,
	)
}

// mqEnvPrefix builds the MQ_* assignment lines prepended to every script so the
// root shell sees the scenario parameters.
func mqEnvPrefix(params *Params) string {
	return fmt.Sprintf(
		"MQ_NQMGRS=%d\nMQ_QUEUES_PER_QM=%d\nMQ_BASE_PORT=%d\nMQ_VERSION=%q\nMQ_DOWNLOAD_URL=%q\nMQ_CHANNEL=%q\nMQ_QM_PREFIX=%q\nMQ_QUEUE_PREFIX=%q\n",
		params.NQmgrs, params.QueuesPerQM, params.BasePort, params.MQVersion,
		params.DownloadURL, params.Channel, params.QMPrefix, params.QueuePrefix,
	)
}

// newMQSetup installs IBM MQ Advanced for Developers and creates the queue
// managers, listeners, channels, and queues.
func newMQSetup(e config.Env, host *remote.Host, params *Params, opts ...pulumi.ResourceOption) (command.Command, error) {
	return runRootScript(e, host, "ibm-mq-setup", mqEnvPrefix(params)+mqSetupScript, opts...)
}

// newMQLoad installs and starts the continuous put/get load systemd service.
func newMQLoad(e config.Env, host *remote.Host, params *Params, opts ...pulumi.ResourceOption) (command.Command, error) {
	return runRootScript(e, host, "ibm-mq-load", mqEnvPrefix(params)+mqLoadScript, opts...)
}

// newMQReadiness blocks until every queue manager is Running and its listener port
// is accepting connections, dumping diagnostics to stderr on failure.
func newMQReadiness(e config.Env, host *remote.Host, params *Params, opts ...pulumi.ResourceOption) (command.Command, error) {
	return runRootScript(e, host, "ibm-mq-readiness", mqEnvPrefix(params)+mqReadinessScript, opts...)
}
