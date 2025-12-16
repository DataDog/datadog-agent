// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gcp

const (
	agentSandboxEnv = "gcp/agent-sandbox"
	agentQaEnv      = "gcp/agent-qa"
)

type environmentDefault struct {
	gcp     gcpProvider
	ddInfra ddInfra
}

type gcpProvider struct {
	project string
	region  string
	zone    string
}

type ddInfra struct {
	defaultInstanceType     string
	defaultNetworkName      string
	defaultSubnetName       string
	defaultVMServiceAccount string
	gke                     ddInfraGKE
	openshift               ddInfraOpenShift
}

type ddInfraGKE struct {
	autopilot bool
}

type ddInfraOpenShift struct {
	nestedVirtualization bool
}

func getEnvironmentDefault(envName string) environmentDefault {
	switch envName {
	case agentSandboxEnv:
		return agentSandboxDefault()
	case agentQaEnv:
		return agentQaDefault()
	default:
		panic("Unknown environment: " + envName)
	}
}

func agentSandboxDefault() environmentDefault {
	return environmentDefault{
		gcp: gcpProvider{
			project: "datadog-agent-sandbox",
			region:  "us-central1",
			zone:    "us-central1-a",
		},
		ddInfra: ddInfra{
			defaultInstanceType:     "e2-standard-2",
			defaultNetworkName:      "datadog-agent-sandbox-us-central1",
			defaultSubnetName:       "datadog-agent-sandbox-us-central1-private",
			defaultVMServiceAccount: "vmserviceaccount@datadog-agent-sandbox.iam.gserviceaccount.com",
			gke:                     ddInfraGKE{autopilot: false},
			openshift:               ddInfraOpenShift{nestedVirtualization: false},
		},
	}
}

func agentQaDefault() environmentDefault {
	return environmentDefault{
		gcp: gcpProvider{
			project: "datadog-agent-qa",
			region:  "us-central1",
			zone:    "us-central1-a",
		},
		ddInfra: ddInfra{
			defaultInstanceType:     "e2-standard-2",
			defaultNetworkName:      "datadog-agent-qa-us-central1",
			defaultSubnetName:       "datadog-agent-qa-us-central1-private",
			defaultVMServiceAccount: "vmserviceaccount@datadog-agent-qa.iam.gserviceaccount.com",
			gke:                     ddInfraGKE{autopilot: false},
			openshift:               ddInfraOpenShift{nestedVirtualization: false},
		},
	}
}
