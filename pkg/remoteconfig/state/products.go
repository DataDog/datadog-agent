// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package state

var validProducts = map[string]struct{}{
	ProductInstallerConfig:              {},
	ProductUpdaterCatalogDD:             {},
	ProductUpdaterAgent:                 {},
	ProductUpdaterTask:                  {},
	ProductActionPlatformRunnerKeys:     {},
	ProductAgentConfig:                  {},
	ProductAgentFailover:                {},
	ProductAgentTask:                    {},
	ProductAgentIntegrations:            {},
	ProductAPMSampling:                  {},
	ProductCWSDD:                        {},
	ProductCWSCustom:                    {},
	ProductCWSProfiles:                  {},
	ProductCSMSideScanning:              {},
	ProductASM:                          {},
	ProductASMFeatures:                  {},
	ProductASMDD:                        {},
	ProductASMData:                      {},
	ProductAPMTracing:                   {},
	ProductSDSRules:                     {},
	ProductSDSAgentConfig:               {},
	ProductLiveDebugging:                {},
	ProductContainerAutoscalingSettings: {},
	ProductContainerAutoscalingValues:   {},
	ProductTesting1:                     {},
	ProductTesting2:                     {},
	ProductOrchestratorK8sCRDs:          {},
	ProductHaAgent:                      {},
	ProductNDMDeviceProfilesCustom:      {},
	ProductMetricControl:                {},
	ProductDataStreamsLiveMessages:      {},
	ProductLiveDebuggingSymbolDB:        {},
	ProductGradualRollout:               {},
	ProductApmPolicies:                  {},
	ProductSyntheticsTest:               {},
	ProductBTFDD:                        {},
	ProductFFEFlags:                     {},
}

const (
	// ProductInstallerConfig is the product used to receive the installer configuration
	ProductInstallerConfig = "INSTALLER_CONFIG"
	// ProductUpdaterCatalogDD is the product used to receive the package catalog from datadog
	ProductUpdaterCatalogDD = "UPDATER_CATALOG_DD"
	// ProductUpdaterAgent is the product used to receive defaults versions to install
	ProductUpdaterAgent = "UPDATER_AGENT"
	// ProductUpdaterTask is the product used to receive tasks to execute
	ProductUpdaterTask = "UPDATER_TASK"
	// ProductActionPlatformRunnerKeys is to receive signing keys for the action platform "private action runner"
	ProductActionPlatformRunnerKeys = "AP_RUNNER_KEYS"
	// ProductAgentConfig is to receive agent configurations, like the log level
	ProductAgentConfig = "AGENT_CONFIG"
	// ProductAgentFailover is to receive the multi-region failover configuration
	ProductAgentFailover = "AGENT_FAILOVER"
	// ProductAgentIntegrations is to receive integrations to schedule
	ProductAgentIntegrations = "AGENT_INTEGRATIONS"
	// ProductAgentTask is to receive agent task instruction, like a flare
	ProductAgentTask = "AGENT_TASK"
	// ProductAPMSampling is the apm sampling product
	ProductAPMSampling = "APM_SAMPLING"
	// ProductCWSDD is the cloud workload security product managed by datadog employees
	ProductCWSDD = "CWS_DD"
	// ProductCWSCustom is the cloud workload security product managed by datadog customers
	ProductCWSCustom = "CWS_CUSTOM"
	// ProductCWSProfiles is the cloud workload security profile product
	ProductCWSProfiles = "CWS_SECURITY_PROFILES"
	// ProductCSMSideScanning is the side scanning product
	ProductCSMSideScanning = "CSM_SIDE_SCANNING"
	// ProductASM is the ASM product used by customers to issue rules configurations
	ProductASM = "ASM"
	// ProductASMFeatures is the ASM product used form ASM activation through remote config
	ProductASMFeatures = "ASM_FEATURES"
	// ProductASMDD is the application security monitoring product managed by datadog employees
	ProductASMDD = "ASM_DD"
	// ProductASMData is the ASM product used to configure WAF rules data
	ProductASMData = "ASM_DATA"
	// ProductAPMTracing is the apm tracing product
	ProductAPMTracing = "APM_TRACING"
	// ProductSDSRules is the SDS definitions product
	ProductSDSRules = "SDS_RULES_DD"
	// ProductSDSAgentConfig is the user SDS configurations product.
	ProductSDSAgentConfig = "SDS_AGENT_CONFIG"
	// ProductLiveDebugging is the dynamic instrumentation product
	ProductLiveDebugging = "LIVE_DEBUGGING"
	// ProductLiveDebuggingSymbolDB is used by the live debugging product for
	// selecting processes to upload symbols to the symbol database.
	ProductLiveDebuggingSymbolDB = "LIVE_DEBUGGING_SYMBOL_DB"
	// ProductContainerAutoscalingSettings receives definition of container autoscaling
	ProductContainerAutoscalingSettings = "CONTAINER_AUTOSCALING_SETTINGS"
	// ProductContainerAutoscalingValues receives values for container autoscaling
	ProductContainerAutoscalingValues = "CONTAINER_AUTOSCALING_VALUES"
	// ProductTesting1 is a product used for testing remote config
	ProductTesting1 = "TESTING1"
	// ProductTesting2 is a product used for testing remote config
	ProductTesting2 = "TESTING2"
	// ProductOrchestratorK8sCRDs receives values for k8s crds
	ProductOrchestratorK8sCRDs = "ORCHESTRATOR_K8S_CRDS"
	// ProductHaAgent is the HA Agent product
	ProductHaAgent = "HA_AGENT"
	// ProductSyntheticsTest is the Synthetics test product
	ProductSyntheticsTest = "SYNTHETIC_TEST"
	// ProductNDMDeviceProfilesCustom receives user-created SNMP profiles for network device monitoring
	ProductNDMDeviceProfilesCustom = "NDM_DEVICE_PROFILES_CUSTOM"
	// ProductMetricControl receives configuration for the metrics control.
	ProductMetricControl = "METRIC_CONTROL"
	// ProductDataStreamsLiveMessages is used for capturing messages from Kafka
	ProductDataStreamsLiveMessages = "DSM_LIVE_MESSAGES"
	// ProductGradualRollout tracks the latest stable release versions for K8s gradual rollout.
	ProductGradualRollout = "K8S_INJECTION_DD"
	// ProductBTFDD accesses a BTF catalog used when the kernel is newer than the system-probe has bundled support for
	ProductBTFDD = "BTF_DD"
	// ProductApmPolicies is the workload selection product
	ProductApmPolicies = "APM_POLICIES"
	// ProductFFEFlags is used for feature flagging experiments remote updates
	ProductFFEFlags = "FFE_FLAGS"
)
