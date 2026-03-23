// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package azure

const (
	sandboxEnv      = "az/sandbox"
	agentSandboxEnv = "az/agent-sandbox"
	agentQaEnv      = "az/agent-qa"
)

type environmentDefault struct {
	azure   azureProvider
	ddInfra ddInfra
}

type azureProvider struct {
	tenantID       string
	subscriptionID string
	location       string
}

type ddInfra struct {
	defaultSubscriptionID    string
	defaultContainerRegistry string
	defaultResourceGroup     string
	defaultVNet              string
	defaultSubnet            string
	defaultSecurityGroup     string
	defaultInstanceType      string
	defaultARMInstanceType   string
	aks                      ddInfraAks
}

type ddInfraAks struct {
	linuxKataNodeGroup bool
}

func getEnvironmentDefault(envName string) environmentDefault {
	switch envName {
	case sandboxEnv:
		return sandboxDefault()
	case agentSandboxEnv:
		return agentSandboxDefault()
	case agentQaEnv:
		return agentQaDefault()
	default:
		panic("Unknown environment: " + envName)
	}
}

func sandboxDefault() environmentDefault {
	return environmentDefault{
		azure: azureProvider{
			tenantID:       "4d3bac44-0230-4732-9e70-cc00736f0a97",
			subscriptionID: "8c56d827-5f07-45ce-8f2b-6c5001db5c6f",
		},
		ddInfra: ddInfra{
			defaultContainerRegistry: "/subscriptions/c767177d-c6fc-47d3-a87e-3ab195f5b99e/resourceGroups/dd-agent-qa/providers/Microsoft.ContainerRegistry/registries/agentqa",
			defaultResourceGroup:     "datadog-agent-testing",
			defaultVNet:              "/subscriptions/8c56d827-5f07-45ce-8f2b-6c5001db5c6f/resourceGroups/datadog-agent-testing/providers/Microsoft.Network/virtualNetworks/default-vnet",
			defaultSubnet:            "/subscriptions/8c56d827-5f07-45ce-8f2b-6c5001db5c6f/resourceGroups/datadog-agent-testing/providers/Microsoft.Network/virtualNetworks/default-vnet/subnets/default-subnet",
			defaultSecurityGroup:     "/subscriptions/8c56d827-5f07-45ce-8f2b-6c5001db5c6f/resourceGroups/datadog-agent-testing/providers/Microsoft.Network/networkSecurityGroups/default",
			defaultInstanceType:      "Standard_D4s_v5",  // Allows nested virtualization for kata runtimes
			defaultARMInstanceType:   "Standard_D4ps_v5", // No azure arm instance supports nested virtualization
			aks: ddInfraAks{
				linuxKataNodeGroup: true,
			},
		},
	}
}

func agentSandboxDefault() environmentDefault {
	return environmentDefault{
		azure: azureProvider{
			tenantID:       "cc0b82f3-7c2e-400b-aec3-40a3d720505b",
			subscriptionID: "9972cab2-9e99-419b-a683-86bfa77b3df1",
			location:       "West US 2",
		},
		ddInfra: ddInfra{
			defaultSubscriptionID:    "9972cab2-9e99-419b-a683-86bfa77b3df1",
			defaultContainerRegistry: "/subscriptions/c767177d-c6fc-47d3-a87e-3ab195f5b99e/resourceGroups/dd-agent-qa/providers/Microsoft.ContainerRegistry/registries/agentqa",
			defaultResourceGroup:     "dd-agent-sandbox",
			defaultVNet:              "/subscriptions/9972cab2-9e99-419b-a683-86bfa77b3df1/resourceGroups/dd-agent-sandbox/providers/Microsoft.Network/virtualNetworks/dd-agent-sandbox",
			defaultSubnet:            "/subscriptions/9972cab2-9e99-419b-a683-86bfa77b3df1/resourceGroups/dd-agent-sandbox/providers/Microsoft.Network/virtualNetworks/dd-agent-sandbox/subnets/dd-agent-sandbox-private",
			defaultSecurityGroup:     "/subscriptions/9972cab2-9e99-419b-a683-86bfa77b3df1/resourceGroups/dd-agent-sandbox/providers/Microsoft.Network/networkSecurityGroups/appgategreen",
			defaultInstanceType:      "Standard_D4s_v5",  // Allows nested virtualization for kata runtimes
			defaultARMInstanceType:   "Standard_D4ps_v5", // No azure arm instance supports nested virtualization
			aks: ddInfraAks{
				linuxKataNodeGroup: true,
			},
		},
	}
}

func agentQaDefault() environmentDefault {
	return environmentDefault{
		azure: azureProvider{
			tenantID:       "cc0b82f3-7c2e-400b-aec3-40a3d720505b",
			subscriptionID: "c767177d-c6fc-47d3-a87e-3ab195f5b99e",
			location:       "West US 2",
		},
		ddInfra: ddInfra{
			defaultSubscriptionID:    "c767177d-c6fc-47d3-a87e-3ab195f5b99e",
			defaultContainerRegistry: "/subscriptions/c767177d-c6fc-47d3-a87e-3ab195f5b99e/resourceGroups/dd-agent-qa/providers/Microsoft.ContainerRegistry/registries/agentqa",
			defaultResourceGroup:     "dd-agent-qa",
			defaultVNet:              "/subscriptions/c767177d-c6fc-47d3-a87e-3ab195f5b99e/resourceGroups/dd-agent-qa/providers/Microsoft.Network/virtualNetworks/dd-agent-qa",
			defaultSubnet:            "/subscriptions/c767177d-c6fc-47d3-a87e-3ab195f5b99e/resourceGroups/dd-agent-qa/providers/Microsoft.Network/virtualNetworks/dd-agent-qa/subnets/dd-agent-qa-private",
			defaultSecurityGroup:     "/subscriptions/c767177d-c6fc-47d3-a87e-3ab195f5b99e/resourceGroups/dd-agent-qa/providers/Microsoft.Network/networkSecurityGroups/appgategreen",
			defaultInstanceType:      "Standard_D4s_v5",  // Allows nested virtualization for kata runtimes
			defaultARMInstanceType:   "Standard_D4ps_v5", // No azure arm instance supports nested virtualization
			aks: ddInfraAks{
				linuxKataNodeGroup: true,
			},
		},
	}
}
