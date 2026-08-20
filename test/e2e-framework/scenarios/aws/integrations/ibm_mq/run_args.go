// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ibm_mq

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
)

const (
	// defaultVMName MUST be "agent-host" so vm.Export() emits stack output
	// dd-Host-aws-agent-host, matching the rendered task module's REMOTE_HOSTNAME
	// ("aws-agent-host"). Never name the VM after the integration.
	defaultVMName = "agent-host"

	// defaultInstanceType matches lab.json capacity_plan.roles[].selected_infra.target
	// (m5.2xlarge: 8 vCPU / 32 GiB). x86_64 ONLY — ibm_mq is in ARM_EXCLUSIONS and has
	// no arm64 build, so the VM must be amd64.
	defaultInstanceType = "m5.2xlarge"

	// defaultStorageSizeGiB matches capacity_plan.roles[].selected_infra.disk_gib.
	defaultStorageSizeGiB = 60

	// IBM MQ workload/topology defaults. These are the queue-count sweep knobs; all
	// are overridable via the ddinfra:ibm_mq/* Pulumi config namespace so the same
	// Go can reach 1000+ total queues (e.g. 5 QMs x 300 queues) without edits.
	defaultNQmgrs      = 2
	defaultQueuesPerQM = 50
	defaultBasePort    = 1414

	// defaultMQVersion / defaultDownloadURL: IBM MQ Advanced for Developers 9.3
	// (free dev edition, LAP acceptance at extract time). The download URL is
	// overridable; the default targets IBM's public developer distribution host.
	defaultMQVersion   = "9.3"
	defaultDownloadURL = "https://public.dhe.ibm.com/ibmdl/export/pub/software/websphere/messaging/mqadv/mqadv_dev930_linux_x86-64.tar.gz"

	// Check-config defaults (ibm_mq.d/conf.yaml instance options, verbatim keys from
	// integrations-core ibm_mq/datadog_checks/ibm_mq/config.py).
	defaultChannel               = "DEV.ADMIN.SVRCONN"
	defaultQMPrefix              = "QM"
	defaultQueuePrefix           = "DEV.QUEUE."
	defaultAutoDiscoverQueues    = true
	defaultCollectResetQueue     = true
	defaultMinCollectionInterval = 15

	// ddinfra:ibm_mq/* Pulumi config keys.
	paramNQmgrs                = "ibm_mq/nQmgrs"
	paramQueuesPerQM           = "ibm_mq/queuesPerQM"
	paramBasePort              = "ibm_mq/basePort"
	paramMQVersion             = "ibm_mq/mqVersion"
	paramDownloadURL           = "ibm_mq/downloadUrl"
	paramAutoDiscoverQueues    = "ibm_mq/autoDiscoverQueues"
	paramQueueRegex            = "ibm_mq/queueRegex"
	paramExplicitQueues        = "ibm_mq/queues"
	paramCollectResetQueue     = "ibm_mq/collectResetQueueMetrics"
	paramMinCollectionInterval = "ibm_mq/minCollectionInterval"
	paramMetricPatternsExclude = "ibm_mq/metricPatternsExclude"
	paramIntegrationProfiling  = "ibm_mq/integrationProfiling"
	paramInternalProfiling     = "ibm_mq/internalProfiling"
)

// Params holds the run parameters for the single-VM IBM MQ scenario.
type Params struct {
	Name            string
	instanceOptions []ec2.VMOption
	// agentOptions is nil when the Agent should not be deployed (ddagent:deploy=false).
	agentOptions []agentparams.Option

	// Workload/topology knobs (queue-count sweep).
	NQmgrs      int
	QueuesPerQM int
	BasePort    int
	MQVersion   string
	DownloadURL string
	Channel     string
	QMPrefix    string
	QueuePrefix string

	// ibm_mq check-config toggles.
	AutoDiscoverQueues    bool
	QueueRegex            string
	ExplicitQueues        []string
	CollectResetQueue     bool
	MinCollectionInterval int
	MetricPatternsExclude []string

	// Profiling toggles: Python/ddtrace integration profiling and Go internal
	// profiling. Both drive datadog.yaml content when enabled.
	IntegrationProfiling bool
	InternalProfiling    bool
}

// ParamsFromEnvironment builds Params from the AWS environment, honoring the
// standard ddagent flags and sizing the host from the capacity plan while exposing
// every workload/check knob through the ddinfra:ibm_mq/* namespace.
func ParamsFromEnvironment(e aws.Environment) *Params {
	p := &Params{
		Name:                  defaultVMName,
		instanceOptions:       []ec2.VMOption{},
		agentOptions:          []agentparams.Option{},
		NQmgrs:                e.GetIntWithDefault(e.InfraConfig, paramNQmgrs, defaultNQmgrs),
		QueuesPerQM:           e.GetIntWithDefault(e.InfraConfig, paramQueuesPerQM, defaultQueuesPerQM),
		BasePort:              e.GetIntWithDefault(e.InfraConfig, paramBasePort, defaultBasePort),
		MQVersion:             e.GetStringWithDefault(e.InfraConfig, paramMQVersion, defaultMQVersion),
		DownloadURL:           e.GetStringWithDefault(e.InfraConfig, paramDownloadURL, defaultDownloadURL),
		Channel:               defaultChannel,
		QMPrefix:              defaultQMPrefix,
		QueuePrefix:           defaultQueuePrefix,
		AutoDiscoverQueues:    e.GetBoolWithDefault(e.InfraConfig, paramAutoDiscoverQueues, defaultAutoDiscoverQueues),
		QueueRegex:            e.GetStringWithDefault(e.InfraConfig, paramQueueRegex, ""),
		ExplicitQueues:        e.GetStringListWithDefault(e.InfraConfig, paramExplicitQueues, nil),
		CollectResetQueue:     e.GetBoolWithDefault(e.InfraConfig, paramCollectResetQueue, defaultCollectResetQueue),
		MinCollectionInterval: e.GetIntWithDefault(e.InfraConfig, paramMinCollectionInterval, defaultMinCollectionInterval),
		MetricPatternsExclude: e.GetStringListWithDefault(e.InfraConfig, paramMetricPatternsExclude, nil),
		IntegrationProfiling:  e.GetBoolWithDefault(e.InfraConfig, paramIntegrationProfiling, false),
		InternalProfiling:     e.GetBoolWithDefault(e.InfraConfig, paramInternalProfiling, false),
	}

	// Pin the instance type + root volume to the capacity-planned target and force
	// x86_64 (RHEL 8 AMD64) — ibm_mq has no arm64 build.
	p.instanceOptions = append(p.instanceOptions,
		ec2.WithInstanceType(defaultInstanceType),
		ec2.WithStorageSize(defaultStorageSizeGiB),
	)

	// Honor ddagent:deploy. Agent version/pipeline/flavor are read from the env by
	// agentparams.NewParams / NewHostAgent.
	if !e.AgentDeploy() {
		p.agentOptions = nil
	}

	return p
}
