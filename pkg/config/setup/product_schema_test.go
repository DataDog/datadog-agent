// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

// Tests of the products declared in the real core and system-probe schemas

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// Every product declared in the real core schema can be enabled on its own
func TestCoreSchemaProducts(t *testing.T) {
	root, err := loadSchema(coreSchemaGetter)
	require.NoError(t, err)
	_, productDependencies := productDefinitions(root)
	require.NotEmpty(t, productDependencies)

	for product := range productDependencies {
		t.Run(product, func(t *testing.T) {
			config := newTestConf(t)
			config.SetInTest("products", []string{product})
			require.NoError(t, ApplyProductEnablement(config))
			assert.Contains(t, ResolvedProducts(), product)
		})
	}
}

// runtimeEnv sets the environment variables the Agent uses to detect where it runs
func runtimeEnv(t *testing.T, runtime string) {
	for _, name := range []string{"DOCKER_DD_AGENT", "KUBERNETES_SERVICE_PORT", "KUBERNETES", "ECS_FARGATE", "AWS_EXECUTION_ENV"} {
		t.Setenv(name, "")
	}
	switch runtime {
	case "kubernetes":
		t.Setenv("DOCKER_DD_AGENT", "true")
		t.Setenv("KUBERNETES_SERVICE_PORT", "443")
	case "fargate":
		t.Setenv("DOCKER_DD_AGENT", "true")
		t.Setenv("ECS_FARGATE", "true")
	}
}

type settings = map[string]interface{}

// settingsFromProducts enables products with the real schemas and returns the core and system-probe settings they set
func settingsFromProducts(t *testing.T, runtime string, products ...string) (settings, settings, error) {
	runtimeEnv(t, runtime)
	config := newTestConf(t)
	config.SetInTest("products", products)
	systemProbe := newTestSystemProbeConf(t)

	if err := ApplyProductEnablement(config); err != nil {
		return nil, nil, err
	}
	require.NoError(t, ApplySystemProbeProductEnablement(systemProbe, config))
	return productLayer(config), productLayer(systemProbe), nil
}

// productLayer returns the flattened settings of the product enablement layer
func productLayer(config pkgconfigmodel.Reader) settings {
	flat := settings{}
	var walk func(prefix string, node map[string]interface{})
	walk = func(prefix string, node map[string]interface{}) {
		for name, value := range node {
			key := name
			if prefix != "" {
				key = prefix + "." + name
			}
			if child, ok := value.(map[string]interface{}); ok {
				walk(key, child)
			} else {
				flat[key] = value
			}
		}
	}
	layer, _ := config.AllSettingsBySource()[pkgconfigmodel.SourceProductEnablement].(map[string]interface{})
	walk("", layer)
	return flat
}

func merge(maps ...settings) settings {
	merged := settings{}
	for _, m := range maps {
		for k, v := range m {
			merged[k] = v
		}
	}
	return merged
}

var (
	apmSettings     = settings{"apm_config.enabled": true, "apm_config.error_tracking_standalone.enabled": false}
	logsSettings    = settings{"logs_enabled": true}
	netflowSettings = settings{
		"network_devices.netflow.enabled": true,
		"network_devices.netflow.listeners": []interface{}{
			map[string]interface{}{"flow_type": "netflow9", "port": 2055},
			map[string]interface{}{"flow_type": "netflow5", "port": 2056},
			map[string]interface{}{"flow_type": "ipfix", "port": 4739},
			map[string]interface{}{"flow_type": "sflow5", "port": 6343},
		},
	}
	vulnerabilitiesSettings = settings{"sbom.enabled": true, "sbom.host.enabled": true, "sbom.container_image.enabled": true}
	jobsSettings            = settings{"djm_config.enabled": true, "expected_tags_duration": 10 * time.Minute}
	usmProtocols            = settings{"service_monitoring_config.http2.enabled": true, "service_monitoring_config.kafka.enabled": true, "service_monitoring_config.postgres.enabled": true, "service_monitoring_config.redis.enabled": true, "service_monitoring_config.tls.nodejs.enabled": true}
)

// The settings each product sets on a Linux host, with the real core and system-probe schemas
func TestCoreSchemaProductDefaultsLinuxHost(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the expectations are for Linux hosts")
	}
	tests := []struct {
		product     string
		core        settings
		systemProbe settings
	}{
		{"infrastructure_monitoring", settings{"infrastructure_mode": "full"}, settings{}},
		{"infrastructure_monitoring_basic", settings{"infrastructure_mode": "basic"}, settings{}},
		{"end_user_device_monitoring", settings{}, settings{}},
		{"container_monitoring", settings{"infrastructure_mode": "full"}, settings{}},
		{"container_live_processes", settings{"infrastructure_mode": "full", "process_config.process_collection.enabled": true}, settings{}},
		{"custom_metrics", settings{}, settings{}},
		{"kubernetes_autoscaling", settings{"infrastructure_mode": "full"}, settings{}},
		{"kubernetes_cluster_autoscaling", settings{"infrastructure_mode": "full"}, settings{}},
		{"gpu_monitoring", settings{"gpu.enabled": true}, settings{"gpu_monitoring.enabled": true}},
		{"gpu_monitoring_advanced", settings{"gpu.enabled": true}, settings{"gpu_monitoring.enabled": true}},
		{"cloud_cost_management", settings{"infrastructure_mode": "cloud_cost_only"}, settings{}},
		{"cloud_cost_management_containers", settings{"infrastructure_mode": "cloud_cost_only"}, settings{}},
		{"disaster_recovery", settings{"multi_region_failover.enabled": true}, settings{}},
		{"network_monitoring", netflowSettings, settings{"network_config.enabled": true, "traceroute.enabled": true}},
		{"cloud_network_monitoring", settings{}, settings{"network_config.enabled": true}},
		{"network_device_monitoring", settings{}, settings{}},
		{"netflow_monitoring", netflowSettings, settings{}},
		{"network_path", settings{}, settings{"traceroute.enabled": true}},
		{"network_path_dynamic_tests", settings{"network_path.connections_monitoring.enabled": true}, settings{"network_config.enabled": true, "traceroute.enabled": true}},
		{"apm", apmSettings, settings{}},
		{"apm_single_step_instrumentation", apmSettings, settings{}},
		{"otlp_apm_ingest", merge(apmSettings, settings{"otlp_config.receiver.protocols.grpc.endpoint": "localhost:4317", "otlp_config.receiver.protocols.http.endpoint": "localhost:4318"}), settings{}},
		{"continuous_profiler", apmSettings, settings{}},
		{"dynamic_instrumentation", apmSettings, settings{}},
		{"dynamic_instrumentation_go", apmSettings, settings{"dynamic_instrumentation.enabled": true}},
		{"data_streams_monitoring", apmSettings, settings{}},
		{"universal_service_monitoring", settings{}, settings{"service_monitoring_config.enabled": true}},
		{"usm_all_protocols", settings{}, merge(usmProtocols, settings{"service_monitoring_config.enabled": true})},
		{"database_monitoring", settings{}, settings{}},
		{"agent_observability", apmSettings, settings{}},
		{"error_tracking_standalone", settings{"apm_config.error_tracking_standalone.enabled": true}, settings{}},
		{"jobs_monitoring", merge(apmSettings, jobsSettings), settings{}},
		{"jobs_monitoring_databricks", merge(apmSettings, jobsSettings, settings{
			"apm_config.obfuscation.credit_cards.keep_values": []string{"databricks_job_id", "databricks_job_run_id", "databricks_task_run_id", "config.spark_app_startTime", "config.spark_databricks_job_parentRunId"},
			"process_config.expvar_port":                      6063,
		}), settings{}},
		{"log_management", logsSettings, settings{}},
		{"log_management_collect_all", logsSettings, settings{}},
		{"observability_pipelines", merge(logsSettings, settings{"observability_pipelines_worker.logs.enabled": true, "observability_pipelines_worker.metrics.enabled": true}), settings{}},
		{"observability_pipelines_logs", merge(logsSettings, settings{"observability_pipelines_worker.logs.enabled": true}), settings{}},
		{"observability_pipelines_metrics", settings{"observability_pipelines_worker.metrics.enabled": true}, settings{}},
		{"cloud_siem", logsSettings, settings{}},
		{"workload_protection", settings{"runtime_security_config.enabled": true}, settings{"runtime_security_config.enabled": true}},
		{"cloud_security", merge(vulnerabilitiesSettings, settings{"compliance_config.enabled": true}), settings{}},
		{"cloud_security_misconfigurations", settings{"compliance_config.enabled": true}, settings{}},
		{"cloud_security_vulnerabilities", vulnerabilitiesSettings, settings{}},
		{"cloud_security_runtime_package_prioritization", merge(vulnerabilitiesSettings, settings{"sbom.enrichment.usage.enabled": true}), settings{}},
		{"app_and_api_protection", apmSettings, settings{}},
		{"app_and_api_protection_proxy_injection", apmSettings, settings{}},
		{"code_security", apmSettings, settings{}},
		{"test_optimization", apmSettings, settings{}},
		{"private_action_runner", settings{"private_action_runner.enabled": true}, settings{}},
		{"workflow_automation", settings{"private_action_runner.enabled": true}, settings{}},
		{"app_builder", settings{"private_action_runner.enabled": true}, settings{}},
	}
	for _, test := range tests {
		t.Run(test.product, func(t *testing.T) {
			core, systemProbe, err := settingsFromProducts(t, "host", test.product)
			require.NoError(t, err)
			assert.Equal(t, test.core, core, "core settings")
			assert.Equal(t, test.systemProbe, systemProbe, "system-probe settings")
		})
	}
}

// Containerized environments get extra settings
func TestCoreSchemaProductDefaultsContainers(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the expectations are for Linux")
	}
	tests := []struct {
		runtime     string
		product     string
		core        settings
		systemProbe settings
	}{
		{"kubernetes", "container_monitoring", settings{"infrastructure_mode": "full", "cluster_agent.enabled": true, "cluster_checks.enabled": true, "leader_election": true, "collect_kubernetes_events": true}, settings{}},
		{"kubernetes", "kubernetes_autoscaling", settings{"infrastructure_mode": "full", "cluster_agent.enabled": true, "cluster_checks.enabled": true, "leader_election": true, "collect_kubernetes_events": true, "autoscaling.workload.enabled": true, "autoscaling.failover.enabled": true, "admission_controller.enabled": true}, settings{}},
		{"kubernetes", "custom_metrics", settings{"dogstatsd_non_local_traffic": true}, settings{}},
		{"kubernetes", "gpu_monitoring", settings{"gpu.enabled": true}, settings{"gpu_monitoring.enabled": false}},
		{"kubernetes", "gpu_monitoring_advanced", settings{"gpu.enabled": true}, settings{"gpu_monitoring.enabled": true}},
		{"kubernetes", "cloud_network_monitoring", settings{"agent_ipc.port": 5009, "agent_ipc.config_refresh_interval": 60}, settings{"network_config.enabled": true}},
		{"kubernetes", "apm_single_step_instrumentation", merge(apmSettings, settings{"apm_config.instrumentation.enabled": true, "admission_controller.enabled": true, "language_detection.enabled": true}), settings{}},
		{"kubernetes", "otlp_apm_ingest", merge(apmSettings, settings{"otlp_config.receiver.protocols.grpc.endpoint": "0.0.0.0:4317", "otlp_config.receiver.protocols.http.endpoint": "0.0.0.0:4318"}), settings{}},
		{"kubernetes", "continuous_profiler", merge(apmSettings, settings{"admission_controller.auto_instrumentation.profiling.enabled": "auto"}), settings{}},
		{"kubernetes", "jobs_monitoring", merge(apmSettings, settings{"djm_config.enabled": true, "expected_tags_duration": time.Duration(0)}), settings{}},
		{"kubernetes", "log_management_collect_all", settings{"logs_enabled": true, "logs_config.k8s_container_use_file": true, "logs_config.container_collect_all": true}, settings{}},
		{"kubernetes", "cloud_security_vulnerabilities", merge(vulnerabilitiesSettings, settings{"sbom.container_image.use_mount": true}), settings{}},
		{"kubernetes", "code_security", merge(apmSettings, settings{"admission_controller.auto_instrumentation.iast.enabled": true, "admission_controller.auto_instrumentation.asm_sca.enabled": true}), settings{}},
		{"kubernetes", "app_and_api_protection_proxy_injection", merge(apmSettings, settings{"admission_controller.auto_instrumentation.asm.enabled": true, "appsec.proxy.enabled": true, "cluster_agent.appsec.injector.enabled": true}), settings{}},
		{"fargate", "workload_protection", settings{"runtime_security_config.enabled": true}, settings{"runtime_security_config.enabled": true, "runtime_security_config.ebpfless.enabled": true}},
		{"fargate", "cloud_network_monitoring", settings{"agent_ipc.port": 5009, "agent_ipc.config_refresh_interval": 60}, settings{"network_config.enabled": true, "network_config.enable_ebpfless": true}},
		{"fargate", "cloud_security", settings{"compliance_config.enabled": false, "sbom.enabled": false, "sbom.host.enabled": false, "sbom.container_image.enabled": false, "sbom.container_image.use_mount": false}, settings{}},
		{"fargate", "custom_metrics", settings{"dogstatsd_non_local_traffic": false}, settings{}},
	}
	for _, test := range tests {
		t.Run(test.runtime+"/"+test.product, func(t *testing.T) {
			core, systemProbe, err := settingsFromProducts(t, test.runtime, test.product)
			require.NoError(t, err)
			assert.Equal(t, test.core, core, "core settings")
			assert.Equal(t, test.systemProbe, systemProbe, "system-probe settings")
		})
	}
}

// Mutually exclusive products refuse to be enabled together
func TestCoreSchemaProductConflicts(t *testing.T) {
	tests := []struct {
		products []string
		expected string
	}{
		{[]string{"apm", "error_tracking_standalone"}, "apm_config.error_tracking_standalone.enabled: apm=false, error_tracking_standalone=true"},
		{[]string{"infrastructure_monitoring", "cloud_cost_management"}, "infrastructure_mode: cloud_cost_management=cloud_cost_only, infrastructure_monitoring=full"},
		// container_monitoring depends on infrastructure_monitoring
		{[]string{"container_monitoring", "infrastructure_monitoring_basic"}, "infrastructure_mode: infrastructure_monitoring=full, infrastructure_monitoring_basic=basic"},
	}
	for _, test := range tests {
		t.Run(strings.Join(test.products, "+"), func(t *testing.T) {
			_, _, err := settingsFromProducts(t, "host", test.products...)
			require.ErrorIs(t, err, ErrProductEnablement)
			assert.Contains(t, err.Error(), test.expected)
		})
	}
}
