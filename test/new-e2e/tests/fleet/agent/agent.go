// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent contains a wrapper around the agent commands for use in tests.
package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
)

// Agent is a cross platform wrapper around the agent commands for use in tests.
type Agent struct {
	t    func() *testing.T
	host *environments.Host
}

// New creates a new instance of Agent.
func New(t func() *testing.T, host *environments.Host) *Agent {
	return &Agent{t: t, host: host}
}

// Version returns the version of the agent.
func (a *Agent) Version() (string, error) {
	status, err := a.Status()
	if err != nil {
		return "", err
	}
	return status.AgentMetadata.AgentVersion, nil
}

// Status returns the status of the agent.
func (a *Agent) Status() (Status, error) {
	rawStatus, err := a.runCommand("status", "--json")
	if err != nil {
		return Status{}, err
	}
	agentStatus := Status{}
	err = json.Unmarshal([]byte(rawStatus), &agentStatus)
	if err != nil {
		return Status{}, err
	}
	return agentStatus, nil
}

// Configuration returns the configuration of the agent.
func (a *Agent) Configuration() (map[string]any, error) {
	rawConfig, err := a.runCommand("config", "--all")
	if err != nil {
		return nil, err
	}
	config := make(map[string]any)
	err = yaml.Unmarshal([]byte(rawConfig), &config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

// InstalledIntegrations returns the installed integrations on the agent and their versions.
func (a *Agent) InstalledIntegrations() (map[string]string, error) {
	rawIntegrations, err := a.runCommand("integration", "freeze")
	if err != nil {
		return nil, err
	}
	integrations := make(map[string]string)
	for integration := range strings.SplitSeq(rawIntegrations, "\n") {
		integration = strings.TrimSpace(integration)
		if strings.HasPrefix(integration, "datadog-") {
			parts := strings.Split(integration, "==")
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid integration format: %s", integration)
			}
			integrations[strings.TrimPrefix(parts[0], "datadog-")] = parts[1]
		}
	}
	return integrations, nil
}

// runCommand runs a command on the remote host.
func (a *Agent) runCommand(command string, args ...string) (string, error) {
	var baseCommand string
	switch a.host.RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		baseCommand = "sudo -u dd-agent datadog-agent"
	case e2eos.WindowsFamily:
		baseCommand = `& "C:\Program Files\Datadog\Datadog Agent\bin\agent.exe"`
	default:
		return "", fmt.Errorf("unsupported OS family: %v", a.host.RemoteHost.OSFamily)
	}

	err := retry.Do(func() error {
		_, err := a.host.RemoteHost.Execute(baseCommand + " config --all")
		return err
	}, retry.Attempts(10), retry.Delay(1*time.Second), retry.DelayType(retry.FixedDelay))
	if err != nil {
		return "", fmt.Errorf("error waiting for agent to be ready: %w", err)
	}
	return a.host.RemoteHost.Execute(fmt.Sprintf("%s %s %s", baseCommand, command, strings.Join(args, " ")))
}

// MustSetExperimentTimeout sets the agent experiment timeout for config and upgrades.
func (a *Agent) MustSetExperimentTimeout(timeout time.Duration) {
	switch a.host.RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		a.host.RemoteHost.MustExecute("sudo mkdir -p /etc/systemd/system/datadog-agent-exp.service.d")
		a.host.RemoteHost.MustExecute(fmt.Sprintf("sudo sh -c 'echo \"[Service]\nEnvironment=EXPERIMENT_TIMEOUT=%ds\" > /etc/systemd/system/datadog-agent-exp.service.d/experiment-timeout.conf'", int(timeout.Seconds())))
	case e2eos.WindowsFamily:
		timeout := int(math.Max(1, timeout.Minutes()))
		err := windowscommon.SetRegistryDWORDValue(a.host.RemoteHost, `HKLM:\SOFTWARE\Datadog\Datadog Agent`, "WatchdogTimeout", timeout)
		require.NoError(a.t(), err)
	default:
		a.t().Fatalf("Unsupported OS family: %v", a.host.RemoteHost.OSFamily)
	}
}

// MustUnsetExperimentTimeout unsets the agent experiment timeout for config and upgrades.
func (a *Agent) MustUnsetExperimentTimeout() {
	switch a.host.RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		a.host.RemoteHost.MustExecute("sudo rm -f /etc/systemd/system/datadog-agent-exp.service.d/experiment-timeout.conf")
	case e2eos.WindowsFamily:
		err := windowscommon.SetRegistryDWORDValue(a.host.RemoteHost, `HKLM:\SOFTWARE\Datadog\Datadog Agent`, "WatchdogTimeout", 60)
		require.NoError(a.t(), err)
	default:
		a.t().Fatalf("Unsupported OS family: %v", a.host.RemoteHost.OSFamily)
	}
}

// Status is the status of the agent.
type Status struct {
	JMXStartupError struct {
		LastError string `json:"LastError"`
		Timestamp int    `json:"Timestamp"`
	} `json:"JMXStartupError"`
	JMXStatus struct {
		Info   interface{} `json:"info"`
		Checks struct {
			InitializedChecks interface{} `json:"initialized_checks"`
			FailedChecks      interface{} `json:"failed_checks"`
		} `json:"checks"`
		Timestamp int `json:"timestamp"`
		Errors    int `json:"errors"`
	} `json:"JMXStatus"`
	NoProxyChanged           []interface{} `json:"NoProxyChanged"`
	NoProxyIgnoredWarningMap []interface{} `json:"NoProxyIgnoredWarningMap"`
	NoProxyUsedInFuture      []interface{} `json:"NoProxyUsedInFuture"`
	TransportWarnings        bool          `json:"TransportWarnings"`
	AgentMetadata            struct {
		AgentStartupTimeMs           int64         `json:"agent_startup_time_ms"`
		AgentVersion                 string        `json:"agent_version"`
		AutoInstrumentationModes     []interface{} `json:"auto_instrumentation_modes"`
		ConfigApmDdURL               string        `json:"config_apm_dd_url"`
		ConfigDdURL                  string        `json:"config_dd_url"`
		ConfigEksFargate             bool          `json:"config_eks_fargate"`
		ConfigID                     string        `json:"config_id"`
		ConfigLogsDdURL              string        `json:"config_logs_dd_url"`
		ConfigLogsSocks5ProxyAddress string        `json:"config_logs_socks5_proxy_address"`
		ConfigNoProxy                []string      `json:"config_no_proxy"`
		ConfigProcessDdURL           string        `json:"config_process_dd_url"`
		ConfigProxyHTTP              string        `json:"config_proxy_http"`
		ConfigProxyHTTPS             string        `json:"config_proxy_https"`
		ConfigSite                   string        `json:"config_site"`
		Diagnostics                  struct {
			Connectivity []struct {
				Status      string `json:"status"`
				Description string `json:"description"`
				Metadata    struct {
					Endpoint string `json:"endpoint"`
				} `json:"metadata,omitempty"`
				Error string `json:"error,omitempty"`
			} `json:"connectivity"`
		} `json:"diagnostics"`
		FeatureApmEnabled                        bool          `json:"feature_apm_enabled"`
		FeatureAutoInstrumentationEnabled        bool          `json:"feature_auto_instrumentation_enabled"`
		FeatureContainerImagesEnabled            bool          `json:"feature_container_images_enabled"`
		FeatureCsmVMContainersEnabled            bool          `json:"feature_csm_vm_containers_enabled"`
		FeatureCsmVMHostsEnabled                 bool          `json:"feature_csm_vm_hosts_enabled"`
		FeatureCspmEnabled                       bool          `json:"feature_cspm_enabled"`
		FeatureCspmHostBenchmarksEnabled         bool          `json:"feature_cspm_host_benchmarks_enabled"`
		FeatureCwsEnabled                        bool          `json:"feature_cws_enabled"`
		FeatureCwsNetworkEnabled                 bool          `json:"feature_cws_network_enabled"`
		FeatureCwsRemoteConfigEnabled            bool          `json:"feature_cws_remote_config_enabled"`
		FeatureCwsSecurityProfilesEnabled        bool          `json:"feature_cws_security_profiles_enabled"`
		FeatureDiscoveryEnabled                  bool          `json:"feature_discovery_enabled"`
		FeatureDynamicInstrumentationEnabled     bool          `json:"feature_dynamic_instrumentation_enabled"`
		FeatureGpuMonitoringEnabled              bool          `json:"feature_gpu_monitoring_enabled"`
		FeatureImdsv2Enabled                     bool          `json:"feature_imdsv2_enabled"`
		FeatureLogsEnabled                       bool          `json:"feature_logs_enabled"`
		FeatureNetworksEnabled                   bool          `json:"feature_networks_enabled"`
		FeatureNetworksHTTPEnabled               bool          `json:"feature_networks_http_enabled"`
		FeatureNetworksHTTPSEnabled              bool          `json:"feature_networks_https_enabled"`
		FeatureOomKillEnabled                    bool          `json:"feature_oom_kill_enabled"`
		FeatureOtlpEnabled                       bool          `json:"feature_otlp_enabled"`
		FeatureProcessEnabled                    bool          `json:"feature_process_enabled"`
		FeatureProcessLanguageDetectionEnabled   bool          `json:"feature_process_language_detection_enabled"`
		FeatureProcessesContainerEnabled         bool          `json:"feature_processes_container_enabled"`
		FeatureRemoteConfigurationEnabled        bool          `json:"feature_remote_configuration_enabled"`
		FeatureTCPQueueLengthEnabled             bool          `json:"feature_tcp_queue_length_enabled"`
		FeatureUsmEnabled                        bool          `json:"feature_usm_enabled"`
		FeatureUsmGoTLSEnabled                   bool          `json:"feature_usm_go_tls_enabled"`
		FeatureUsmHTTP2Enabled                   bool          `json:"feature_usm_http2_enabled"`
		FeatureUsmIstioEnabled                   bool          `json:"feature_usm_istio_enabled"`
		FeatureUsmKafkaEnabled                   bool          `json:"feature_usm_kafka_enabled"`
		FeatureUsmPostgresEnabled                bool          `json:"feature_usm_postgres_enabled"`
		FeatureUsmRedisEnabled                   bool          `json:"feature_usm_redis_enabled"`
		FeatureWindowsCrashDetectionEnabled      bool          `json:"feature_windows_crash_detection_enabled"`
		FipsMode                                 bool          `json:"fips_mode"`
		Flavor                                   string        `json:"flavor"`
		FleetPoliciesApplied                     []interface{} `json:"fleet_policies_applied"`
		HostnameSource                           string        `json:"hostname_source"`
		InfrastructureMode                       string        `json:"infrastructure_mode"`
		InstallMethodInstallerVersion            string        `json:"install_method_installer_version"`
		InstallMethodTool                        string        `json:"install_method_tool"`
		InstallMethodToolVersion                 string        `json:"install_method_tool_version"`
		SystemProbeCoreEnabled                   bool          `json:"system_probe_core_enabled"`
		SystemProbeGatewayLookupEnabled          bool          `json:"system_probe_gateway_lookup_enabled"`
		SystemProbeKernelHeadersDownloadEnabled  bool          `json:"system_probe_kernel_headers_download_enabled"`
		SystemProbeMaxConnectionsPerMessage      int           `json:"system_probe_max_connections_per_message"`
		SystemProbePrebuiltFallbackEnabled       bool          `json:"system_probe_prebuilt_fallback_enabled"`
		SystemProbeProtocolClassificationEnabled bool          `json:"system_probe_protocol_classification_enabled"`
		SystemProbeRootNamespaceEnabled          bool          `json:"system_probe_root_namespace_enabled"`
		SystemProbeRuntimeCompilationEnabled     bool          `json:"system_probe_runtime_compilation_enabled"`
		SystemProbeTelemetryEnabled              bool          `json:"system_probe_telemetry_enabled"`
		SystemProbeTrackTCP4Connections          bool          `json:"system_probe_track_tcp_4_connections"`
		SystemProbeTrackTCP6Connections          bool          `json:"system_probe_track_tcp_6_connections"`
		SystemProbeTrackUDP4Connections          bool          `json:"system_probe_track_udp_4_connections"`
		SystemProbeTrackUDP6Connections          bool          `json:"system_probe_track_udp_6_connections"`
	} `json:"agent_metadata"`
	AgentStartNano  int64 `json:"agent_start_nano"`
	AggregatorStats struct {
		ChecksHistogramBucketMetricSample int `json:"ChecksHistogramBucketMetricSample"`
		ChecksMetricSample                int `json:"ChecksMetricSample"`
		DogstatsdContexts                 int `json:"DogstatsdContexts"`
		DogstatsdContextsByMtype          struct {
			Count              int `json:"Count"`
			CountWithTimestamp int `json:"CountWithTimestamp"`
			Counter            int `json:"Counter"`
			Distribution       int `json:"Distribution"`
			Gauge              int `json:"Gauge"`
			GaugeWithTimestamp int `json:"GaugeWithTimestamp"`
			Histogram          int `json:"Histogram"`
			Historate          int `json:"Historate"`
			MonotonicCount     int `json:"MonotonicCount"`
			Rate               int `json:"Rate"`
			Set                int `json:"Set"`
		} `json:"DogstatsdContextsByMtype"`
		DogstatsdMetricSample int `json:"DogstatsdMetricSample"`
		Event                 int `json:"Event"`
		EventPlatformEvents   struct {
		} `json:"EventPlatformEvents"`
		EventPlatformEventsErrors struct {
		} `json:"EventPlatformEventsErrors"`
		EventsFlushErrors int `json:"EventsFlushErrors"`
		EventsFlushed     int `json:"EventsFlushed"`
		Flush             struct {
			ChecksMetricSampleFlushTime struct {
				FlushIndex int    `json:"FlushIndex"`
				Flushes    []int  `json:"Flushes"`
				LastFlush  int    `json:"LastFlush"`
				Name       string `json:"Name"`
			} `json:"ChecksMetricSampleFlushTime"`
			EventFlushTime struct {
				FlushIndex int    `json:"FlushIndex"`
				Flushes    []int  `json:"Flushes"`
				LastFlush  int    `json:"LastFlush"`
				Name       string `json:"Name"`
			} `json:"EventFlushTime"`
			MainFlushTime struct {
				FlushIndex int    `json:"FlushIndex"`
				Flushes    []int  `json:"Flushes"`
				LastFlush  int    `json:"LastFlush"`
				Name       string `json:"Name"`
			} `json:"MainFlushTime"`
			ManifestsTime struct {
				FlushIndex int    `json:"FlushIndex"`
				Flushes    []int  `json:"Flushes"`
				LastFlush  int    `json:"LastFlush"`
				Name       string `json:"Name"`
			} `json:"ManifestsTime"`
			MetricSketchFlushTime struct {
				FlushIndex int    `json:"FlushIndex"`
				Flushes    []int  `json:"Flushes"`
				LastFlush  int    `json:"LastFlush"`
				Name       string `json:"Name"`
			} `json:"MetricSketchFlushTime"`
			ServiceCheckFlushTime struct {
				FlushIndex int    `json:"FlushIndex"`
				Flushes    []int  `json:"Flushes"`
				LastFlush  int    `json:"LastFlush"`
				Name       string `json:"Name"`
			} `json:"ServiceCheckFlushTime"`
		} `json:"Flush"`
		FlushCount struct {
			Events struct {
				FlushIndex int    `json:"FlushIndex"`
				Flushes    []int  `json:"Flushes"`
				LastFlush  int    `json:"LastFlush"`
				Name       string `json:"Name"`
			} `json:"Events"`
			Manifests struct {
				FlushIndex int    `json:"FlushIndex"`
				Flushes    []int  `json:"Flushes"`
				LastFlush  int    `json:"LastFlush"`
				Name       string `json:"Name"`
			} `json:"Manifests"`
			Series struct {
				FlushIndex int    `json:"FlushIndex"`
				Flushes    []int  `json:"Flushes"`
				LastFlush  int    `json:"LastFlush"`
				Name       string `json:"Name"`
			} `json:"Series"`
			ServiceChecks struct {
				FlushIndex int    `json:"FlushIndex"`
				Flushes    []int  `json:"Flushes"`
				LastFlush  int    `json:"LastFlush"`
				Name       string `json:"Name"`
			} `json:"ServiceChecks"`
			Sketches struct {
				FlushIndex int    `json:"FlushIndex"`
				Flushes    []int  `json:"Flushes"`
				LastFlush  int    `json:"LastFlush"`
				Name       string `json:"Name"`
			} `json:"Sketches"`
		} `json:"FlushCount"`
		HostnameUpdate int `json:"HostnameUpdate"`
		MetricTags     struct {
			Series struct {
				Above100 int `json:"Above100"`
				Above90  int `json:"Above90"`
			} `json:"Series"`
			Sketches struct {
				Above100 int `json:"Above100"`
				Above90  int `json:"Above90"`
			} `json:"Sketches"`
		} `json:"MetricTags"`
		NumberOfFlush               int `json:"NumberOfFlush"`
		OrchestratorManifests       int `json:"OrchestratorManifests"`
		OrchestratorManifestsErrors int `json:"OrchestratorManifestsErrors"`
		OrchestratorMetadata        int `json:"OrchestratorMetadata"`
		OrchestratorMetadataErrors  int `json:"OrchestratorMetadataErrors"`
		SeriesFlushErrors           int `json:"SeriesFlushErrors"`
		SeriesFlushed               int `json:"SeriesFlushed"`
		ServiceCheck                int `json:"ServiceCheck"`
		ServiceCheckFlushErrors     int `json:"ServiceCheckFlushErrors"`
		ServiceCheckFlushed         int `json:"ServiceCheckFlushed"`
		SketchesFlushErrors         int `json:"SketchesFlushErrors"`
		SketchesFlushed             int `json:"SketchesFlushed"`
	} `json:"aggregatorStats"`
	ApmStats struct {
		Cmdline []string `json:"cmdline"`
		Config  struct {
			AgentVersion                string `json:"AgentVersion"`
			AnalyzedRateByServiceLegacy struct {
			} `json:"AnalyzedRateByServiceLegacy"`
			AnalyzedSpansByService struct {
			} `json:"AnalyzedSpansByService"`
			AdditionalProfileTags    map[string]string `json:"AdditionalProfileTags"`
			BucketInterval           int64             `json:"BucketInterval"`
			ClientStatsFlushInterval int               `json:"ClientStatsFlushInterval"`
			ComputeStatsBySpanKind   bool              `json:"ComputeStatsBySpanKind"`
			ConfigPath               string            `json:"ConfigPath"`
			ConnectionLimit          int               `json:"ConnectionLimit"`
			ConnectionResetInterval  int               `json:"ConnectionResetInterval"`
			ContainerProcRoot        string            `json:"ContainerProcRoot"`
			DDAgentBin               string            `json:"DDAgentBin"`
			DebugServerPort          int               `json:"DebugServerPort"`
			DebuggerIntakeProxy      struct {
				DDURL string `json:"DDURL"`
			} `json:"DebuggerIntakeProxy"`
			DebuggerProxy struct {
				DDURL string `json:"DDURL"`
			} `json:"DebuggerProxy"`
			DecoderTimeout int    `json:"DecoderTimeout"`
			Decoders       int    `json:"Decoders"`
			DefaultEnv     string `json:"DefaultEnv"`
			EVPProxy       struct {
				AdditionalEndpoints interface{} `json:"AdditionalEndpoints"`
				DDURL               string      `json:"DDURL"`
				Enabled             bool        `json:"Enabled"`
				MaxPayloadSize      int         `json:"MaxPayloadSize"`
				ReceiverTimeout     int         `json:"ReceiverTimeout"`
			} `json:"EVPProxy"`
			Enabled   bool `json:"Enabled"`
			Endpoints []struct {
				Host    string `json:"Host"`
				NoProxy bool   `json:"NoProxy"`
			} `json:"Endpoints"`
			ErrorTPS                int         `json:"ErrorTPS"`
			ErrorTrackingStandalone bool        `json:"ErrorTrackingStandalone"`
			ExtraAggregators        interface{} `json:"ExtraAggregators"`
			ExtraSampleRate         int         `json:"ExtraSampleRate"`
			FargateOrchestrator     string      `json:"FargateOrchestrator"`
			Features                struct {
			} `json:"Features"`
			GUIPort    string `json:"GUIPort"`
			GitCommit  string `json:"GitCommit"`
			GlobalTags struct {
			} `json:"GlobalTags"`
			Hostname string `json:"Hostname"`
			Ignore   struct {
			} `json:"Ignore"`
			InstallSignature struct {
				InstallID   string `json:"install_id"`
				InstallTime int    `json:"install_time"`
				InstallType string `json:"install_type"`
			} `json:"InstallSignature"`
			LambdaFunctionName    string      `json:"LambdaFunctionName"`
			LogFilePath           string      `json:"LogFilePath"`
			MRFFailoverAPMDefault bool        `json:"MRFFailoverAPMDefault"`
			MRFFailoverAPMRC      interface{} `json:"MRFFailoverAPMRC"`
			MaxCPU                float64     `json:"MaxCPU"`
			MaxCatalogEntries     int         `json:"MaxCatalogEntries"`
			MaxConnections        int         `json:"MaxConnections"`
			MaxEPS                int         `json:"MaxEPS"`
			MaxMemory             int         `json:"MaxMemory"`
			MaxRemoteTPS          int         `json:"MaxRemoteTPS"`
			MaxRequestBytes       int         `json:"MaxRequestBytes"`
			MaxResourceLen        int         `json:"MaxResourceLen"`
			MaxSenderRetries      int         `json:"MaxSenderRetries"`
			OTLPReceiver          struct {
				AttributesTranslator struct {
				} `json:"AttributesTranslator"`
				BindHost               string `json:"BindHost"`
				GRPCPort               int    `json:"GRPCPort"`
				GrpcMaxRecvMsgSizeMib  int    `json:"GrpcMaxRecvMsgSizeMib"`
				MaxRequestBytes        int    `json:"MaxRequestBytes"`
				ProbabilisticSampling  int    `json:"ProbabilisticSampling"`
				SpanNameAsResourceName bool   `json:"SpanNameAsResourceName"`
				SpanNameRemappings     struct {
				} `json:"SpanNameRemappings"`
			} `json:"OTLPReceiver"`
			Obfuscation struct {
				Cache struct {
					Enabled bool `json:"Enabled"`
					MaxSize int  `json:"MaxSize"`
				} `json:"Cache"`
				CreditCards struct {
					Enabled    bool          `json:"Enabled"`
					KeepValues []interface{} `json:"KeepValues"`
					Luhn       bool          `json:"Luhn"`
				} `json:"CreditCards"`
				ES struct {
					Enabled            bool          `json:"Enabled"`
					KeepValues         []interface{} `json:"KeepValues"`
					ObfuscateSQLValues []interface{} `json:"ObfuscateSQLValues"`
				} `json:"ES"`
				HTTP struct {
					RemovePathDigits  bool `json:"remove_path_digits"`
					RemoveQueryString bool `json:"remove_query_string"`
				} `json:"HTTP"`
				Memcached struct {
					Enabled     bool `json:"Enabled"`
					KeepCommand bool `json:"KeepCommand"`
				} `json:"Memcached"`
				Mongo struct {
					Enabled            bool          `json:"Enabled"`
					KeepValues         []interface{} `json:"KeepValues"`
					ObfuscateSQLValues []interface{} `json:"ObfuscateSQLValues"`
				} `json:"Mongo"`
				OpenSearch struct {
					Enabled            bool          `json:"Enabled"`
					KeepValues         []interface{} `json:"KeepValues"`
					ObfuscateSQLValues []interface{} `json:"ObfuscateSQLValues"`
				} `json:"OpenSearch"`
				Redis struct {
					Enabled       bool `json:"Enabled"`
					RemoveAllArgs bool `json:"RemoveAllArgs"`
				} `json:"Redis"`
				RemoveStackTraces bool `json:"RemoveStackTraces"`
				SQLExecPlan       struct {
					Enabled            bool          `json:"Enabled"`
					KeepValues         []interface{} `json:"KeepValues"`
					ObfuscateSQLValues []interface{} `json:"ObfuscateSQLValues"`
				} `json:"SQLExecPlan"`
				SQLExecPlanNormalize struct {
					Enabled            bool          `json:"Enabled"`
					KeepValues         []interface{} `json:"KeepValues"`
					ObfuscateSQLValues []interface{} `json:"ObfuscateSQLValues"`
				} `json:"SQLExecPlanNormalize"`
				Valkey struct {
					Enabled       bool `json:"Enabled"`
					RemoveAllArgs bool `json:"RemoveAllArgs"`
				} `json:"Valkey"`
			} `json:"Obfuscation"`
			OpenLineageProxy struct {
				APIVersion          int         `json:"APIVersion"`
				AdditionalEndpoints interface{} `json:"AdditionalEndpoints"`
				DDURL               string      `json:"DDURL"`
				Enabled             bool        `json:"Enabled"`
			} `json:"OpenLineageProxy"`
			PeerTags                               interface{} `json:"PeerTags"`
			PeerTagsAggregation                    bool        `json:"PeerTagsAggregation"`
			PipeBufferSize                         int         `json:"PipeBufferSize"`
			PipeSecurityDescriptor                 string      `json:"PipeSecurityDescriptor"`
			ProbabilisticSamplerEnabled            bool        `json:"ProbabilisticSamplerEnabled"`
			ProbabilisticSamplerHashSeed           int         `json:"ProbabilisticSamplerHashSeed"`
			ProbabilisticSamplerSamplingPercentage int         `json:"ProbabilisticSamplerSamplingPercentage"`
			ProfilingProxy                         struct {
				AdditionalEndpoints interface{} `json:"AdditionalEndpoints"`
				DDURL               string      `json:"DDURL"`
				ReceiverTimeout     int         `json:"ReceiverTimeout"`
			} `json:"ProfilingProxy"`
			ProxyURL                  interface{} `json:"ProxyURL"`
			RareSamplerCardinality    int         `json:"RareSamplerCardinality"`
			RareSamplerCooldownPeriod int64       `json:"RareSamplerCooldownPeriod"`
			RareSamplerEnabled        bool        `json:"RareSamplerEnabled"`
			RareSamplerTPS            int         `json:"RareSamplerTPS"`
			ReceiverEnabled           bool        `json:"ReceiverEnabled"`
			ReceiverHost              string      `json:"ReceiverHost"`
			ReceiverPort              int         `json:"ReceiverPort"`
			ReceiverSocket            string      `json:"ReceiverSocket"`
			ReceiverTimeout           int         `json:"ReceiverTimeout"`
			RejectTags                interface{} `json:"RejectTags"`
			RejectTagsRegex           interface{} `json:"RejectTagsRegex"`
			ReplaceTags               interface{} `json:"ReplaceTags"`
			RequireTags               interface{} `json:"RequireTags"`
			RequireTagsRegex          interface{} `json:"RequireTagsRegex"`
			SQLObfuscationMode        string      `json:"SQLObfuscationMode"`
			Site                      string      `json:"Site"`
			SkipSSLValidation         bool        `json:"SkipSSLValidation"`
			StatsWriter               struct {
				ConnectionLimit    int `json:"ConnectionLimit"`
				FlushPeriodSeconds int `json:"FlushPeriodSeconds"`
				QueueSize          int `json:"QueueSize"`
			} `json:"StatsWriter"`
			StatsdEnabled  bool   `json:"StatsdEnabled"`
			StatsdHost     string `json:"StatsdHost"`
			StatsdPipeName string `json:"StatsdPipeName"`
			StatsdPort     int    `json:"StatsdPort"`
			StatsdSocket   string `json:"StatsdSocket"`
			SymDBProxy     struct {
				DDURL string `json:"DDURL"`
			} `json:"SymDBProxy"`
			SynchronousFlushing bool `json:"SynchronousFlushing"`
			TargetTPS           int  `json:"TargetTPS"`
			TelemetryConfig     struct {
				Enabled   bool `json:"Enabled"`
				Endpoints []struct {
					Host    string `json:"Host"`
					NoProxy bool   `json:"NoProxy"`
				} `json:"Endpoints"`
			} `json:"TelemetryConfig"`
			TraceBuffer int `json:"TraceBuffer"`
			TraceWriter struct {
				ConnectionLimit    int `json:"ConnectionLimit"`
				FlushPeriodSeconds int `json:"FlushPeriodSeconds"`
				QueueSize          int `json:"QueueSize"`
			} `json:"TraceWriter"`
			WatchdogInterval int64  `json:"WatchdogInterval"`
			WindowsPipeName  string `json:"WindowsPipeName"`
		} `json:"config"`
		Memstats struct {
			Alloc       int `json:"Alloc"`
			BuckHashSys int `json:"BuckHashSys"`
			BySize      []struct {
				Frees   int `json:"Frees"`
				Mallocs int `json:"Mallocs"`
				Size    int `json:"Size"`
			} `json:"BySize"`
			DebugGC       bool          `json:"DebugGC"`
			EnableGC      bool          `json:"EnableGC"`
			Frees         int           `json:"Frees"`
			GCCPUFraction float64       `json:"GCCPUFraction"`
			GCSys         int           `json:"GCSys"`
			HeapAlloc     int           `json:"HeapAlloc"`
			HeapIdle      int           `json:"HeapIdle"`
			HeapInuse     int           `json:"HeapInuse"`
			HeapObjects   int           `json:"HeapObjects"`
			HeapReleased  int           `json:"HeapReleased"`
			HeapSys       int           `json:"HeapSys"`
			LastGC        int64         `json:"LastGC"`
			Lookups       int           `json:"Lookups"`
			MCacheInuse   int           `json:"MCacheInuse"`
			MCacheSys     int           `json:"MCacheSys"`
			MSpanInuse    int           `json:"MSpanInuse"`
			MSpanSys      int           `json:"MSpanSys"`
			Mallocs       int           `json:"Mallocs"`
			NextGC        int           `json:"NextGC"`
			NumForcedGC   int           `json:"NumForcedGC"`
			NumGC         int           `json:"NumGC"`
			OtherSys      int           `json:"OtherSys"`
			PauseEnd      []interface{} `json:"PauseEnd"`
			PauseNs       []int         `json:"PauseNs"`
			PauseTotalNs  int           `json:"PauseTotalNs"`
			StackInuse    int           `json:"StackInuse"`
			StackSys      int           `json:"StackSys"`
			Sys           int           `json:"Sys"`
			TotalAlloc    int           `json:"TotalAlloc"`
		} `json:"memstats"`
		Pid           string `json:"pid"`
		Ratebyservice struct {
		} `json:"ratebyservice"`
		RatebyserviceFiltered struct {
		} `json:"ratebyservice_filtered"`
		Receiver    []interface{} `json:"receiver"`
		StatsWriter struct {
			Bytes          int `json:"Bytes"`
			ClientPayloads int `json:"ClientPayloads"`
			Errors         int `json:"Errors"`
			Payloads       int `json:"Payloads"`
			Retries        int `json:"Retries"`
			Splits         int `json:"Splits"`
			StatsBuckets   int `json:"StatsBuckets"`
			StatsEntries   int `json:"StatsEntries"`
		} `json:"stats_writer"`
		TraceWriter struct {
			Bytes             int `json:"Bytes"`
			BytesUncompressed int `json:"BytesUncompressed"`
			Errors            int `json:"Errors"`
			Events            int `json:"Events"`
			Payloads          int `json:"Payloads"`
			Retries           int `json:"Retries"`
			SingleMaxSize     int `json:"SingleMaxSize"`
			Spans             int `json:"Spans"`
			Traces            int `json:"Traces"`
		} `json:"trace_writer"`
		Uptime  int `json:"uptime"`
		Version struct {
			GitCommit string `json:"GitCommit"`
			Version   string `json:"Version"`
		} `json:"version"`
		Watchdog struct {
			CPU struct {
				UserAvg float64 `json:"UserAvg"`
			} `json:"CPU"`
			Mem struct {
				Alloc int `json:"Alloc"`
			} `json:"Mem"`
		} `json:"watchdog"`
	} `json:"apmStats"`
	AutoConfigStats struct {
		ConfigErrors struct {
		} `json:"ConfigErrors"`
		ResolveWarnings struct {
		} `json:"ResolveWarnings"`
	} `json:"autoConfigStats"`
	AutodiscoverySubnets []interface{} `json:"autodiscoverySubnets"`
	BuildArch            string        `json:"build_arch"`
	CheckSchedulerStats  struct {
		LoaderErrors struct {
		} `json:"LoaderErrors"`
		RunErrors struct {
		} `json:"RunErrors"`
	} `json:"checkSchedulerStats"`
	ConfFile string `json:"conf_file"`
	Config   struct {
		AdditionalChecksd  string `json:"additional_checksd"`
		ConfdPath          string `json:"confd_path"`
		FipsLocalAddress   string `json:"fips_local_address"`
		FipsPortRangeStart string `json:"fips_port_range_start"`
		FipsProxyEnabled   string `json:"fips_proxy_enabled"`
		LogFile            string `json:"log_file"`
		LogLevel           string `json:"log_level"`
	} `json:"config"`
	DiscoverySubnets []interface{} `json:"discoverySubnets"`
	DogstatsdStats   struct {
		EventPackets             int `json:"EventPackets"`
		EventParseErrors         int `json:"EventParseErrors"`
		MetricPackets            int `json:"MetricPackets"`
		MetricParseErrors        int `json:"MetricParseErrors"`
		ServiceCheckPackets      int `json:"ServiceCheckPackets"`
		ServiceCheckParseErrors  int `json:"ServiceCheckParseErrors"`
		UDPBytes                 int `json:"UdpBytes"`
		UDPPacketReadingErrors   int `json:"UdpPacketReadingErrors"`
		UDPPackets               int `json:"UdpPackets"`
		UdsBytes                 int `json:"UdsBytes"`
		UdsOriginDetectionErrors int `json:"UdsOriginDetectionErrors"`
		UdsPacketReadingErrors   int `json:"UdsPacketReadingErrors"`
		UdsPackets               int `json:"UdsPackets"`
		UnterminatedMetricErrors int `json:"UnterminatedMetricErrors"`
	} `json:"dogstatsdStats"`
	Enabled        bool `json:"enabled"`
	EndpointsInfos struct {
		HTTPSAppDatadoghqCom []string `json:"https://app.datadoghq.com."`
	} `json:"endpointsInfos"`
	ExtraConfFile         []interface{} `json:"extra_conf_file"`
	FipsStatus            string        `json:"fips_status"`
	Flavor                string        `json:"flavor"`
	FleetAutomationStatus struct {
		FleetAutomationEnabled  bool `json:"fleetAutomationEnabled"`
		InstallerRunning        bool `json:"installerRunning"`
		RemoteManagementEnabled bool `json:"remoteManagementEnabled"`
	} `json:"fleetAutomationStatus"`
	ForwarderStats struct {
		APIKeyFailure struct {
			APIKeyEndingWithXXXXX string `json:"API key ending with XXXXX"`
		} `json:"APIKeyFailure"`
		APIKeyStatus struct {
		} `json:"APIKeyStatus"`
		FileStorage struct {
			CurrentSizeInBytes             int `json:"CurrentSizeInBytes"`
			DeserializeCount               int `json:"DeserializeCount"`
			DeserializeErrorsCount         int `json:"DeserializeErrorsCount"`
			DeserializeTransactionsCount   int `json:"DeserializeTransactionsCount"`
			FileSize                       int `json:"FileSize"`
			FilesCount                     int `json:"FilesCount"`
			FilesRemovedCount              int `json:"FilesRemovedCount"`
			PointsDroppedCount             int `json:"PointsDroppedCount"`
			SerializeCount                 int `json:"SerializeCount"`
			StartupReloadedRetryFilesCount int `json:"StartupReloadedRetryFilesCount"`
		} `json:"FileStorage"`
		RemovalPolicy struct {
			FilesFromUnknownDomainCount int `json:"FilesFromUnknownDomainCount"`
			NewRemovalPolicyCount       int `json:"NewRemovalPolicyCount"`
			OutdatedFilesCount          int `json:"OutdatedFilesCount"`
			RegisteredDomainCount       int `json:"RegisteredDomainCount"`
		} `json:"RemovalPolicy"`
		TransactionContainer struct {
			CurrentMemSizeInBytes    int `json:"CurrentMemSizeInBytes"`
			ErrorsCount              int `json:"ErrorsCount"`
			PointsDroppedCount       int `json:"PointsDroppedCount"`
			TransactionsCount        int `json:"TransactionsCount"`
			TransactionsDroppedCount int `json:"TransactionsDroppedCount"`
		} `json:"TransactionContainer"`
		Transactions struct {
			Cluster            int `json:"Cluster"`
			ClusterRole        int `json:"ClusterRole"`
			ClusterRoleBinding int `json:"ClusterRoleBinding"`
			ConnectionEvents   struct {
				ConnectSuccess int `json:"ConnectSuccess"`
				DNSSuccess     int `json:"DNSSuccess"`
			} `json:"ConnectionEvents"`
			CronJob                  int `json:"CronJob"`
			CustomResource           int `json:"CustomResource"`
			CustomResourceDefinition int `json:"CustomResourceDefinition"`
			DaemonSet                int `json:"DaemonSet"`
			Deployment               int `json:"Deployment"`
			Dropped                  int `json:"Dropped"`
			DroppedByEndpoint        struct {
				CheckRunV1       int `json:"check_run_v1"`
				Intake           int `json:"intake"`
				MetadataV1       int `json:"metadata_v1"`
				ProcessDiscovery int `json:"process_discovery"`
				SeriesV2         int `json:"series_v2"`
			} `json:"DroppedByEndpoint"`
			ECSTask       int `json:"ECSTask"`
			EndpointSlice int `json:"EndpointSlice"`
			Errors        int `json:"Errors"`
			ErrorsByType  struct {
				ConnectionErrors   int `json:"ConnectionErrors"`
				DNSErrors          int `json:"DNSErrors"`
				SentRequestErrors  int `json:"SentRequestErrors"`
				TLSErrors          int `json:"TLSErrors"`
				WroteRequestErrors int `json:"WroteRequestErrors"`
			} `json:"ErrorsByType"`
			HTTPErrors       int `json:"HTTPErrors"`
			HTTPErrorsByCode struct {
				Num403 int `json:"403"`
			} `json:"HTTPErrorsByCode"`
			HighPriorityQueueFull   int `json:"HighPriorityQueueFull"`
			HorizontalPodAutoscaler int `json:"HorizontalPodAutoscaler"`
			Ingress                 int `json:"Ingress"`
			InputBytesByEndpoint    struct {
				CheckRunV1       int `json:"check_run_v1"`
				Intake           int `json:"intake"`
				MetadataV1       int `json:"metadata_v1"`
				ProcessDiscovery int `json:"process_discovery"`
				SeriesV2         int `json:"series_v2"`
			} `json:"InputBytesByEndpoint"`
			InputCountByEndpoint struct {
				CheckRunV1       int `json:"check_run_v1"`
				Intake           int `json:"intake"`
				MetadataV1       int `json:"metadata_v1"`
				ProcessDiscovery int `json:"process_discovery"`
				SeriesV2         int `json:"series_v2"`
			} `json:"InputCountByEndpoint"`
			Job                   int `json:"Job"`
			LimitRange            int `json:"LimitRange"`
			Namespace             int `json:"Namespace"`
			NetworkPolicy         int `json:"NetworkPolicy"`
			Node                  int `json:"Node"`
			OrchestratorManifest  int `json:"OrchestratorManifest"`
			PersistentVolume      int `json:"PersistentVolume"`
			PersistentVolumeClaim int `json:"PersistentVolumeClaim"`
			Pod                   int `json:"Pod"`
			PodDisruptionBudget   int `json:"PodDisruptionBudget"`
			ReplicaSet            int `json:"ReplicaSet"`
			Requeued              int `json:"Requeued"`
			RequeuedByEndpoint    struct {
			} `json:"RequeuedByEndpoint"`
			Retried           int `json:"Retried"`
			RetriedByEndpoint struct {
			} `json:"RetriedByEndpoint"`
			RetryQueueSize    int `json:"RetryQueueSize"`
			Role              int `json:"Role"`
			RoleBinding       int `json:"RoleBinding"`
			Service           int `json:"Service"`
			ServiceAccount    int `json:"ServiceAccount"`
			StatefulSet       int `json:"StatefulSet"`
			StorageClass      int `json:"StorageClass"`
			Success           int `json:"Success"`
			SuccessByEndpoint struct {
				CheckRunV1       int `json:"check_run_v1"`
				Connections      int `json:"connections"`
				Container        int `json:"container"`
				EventsV2         int `json:"events_v2"`
				HostMetadataV2   int `json:"host_metadata_v2"`
				Intake           int `json:"intake"`
				Orchestrator     int `json:"orchestrator"`
				Process          int `json:"process"`
				Rtcontainer      int `json:"rtcontainer"`
				Rtprocess        int `json:"rtprocess"`
				SeriesV1         int `json:"series_v1"`
				SeriesV2         int `json:"series_v2"`
				ServicesChecksV2 int `json:"services_checks_v2"`
				SketchesV1       int `json:"sketches_v1"`
				SketchesV2       int `json:"sketches_v2"`
				ValidateV1       int `json:"validate_v1"`
			} `json:"SuccessByEndpoint"`
			SuccessBytesByEndpoint struct {
			} `json:"SuccessBytesByEndpoint"`
			VerticalPodAutoscaler int `json:"VerticalPodAutoscaler"`
		} `json:"Transactions"`
	} `json:"forwarderStats"`
	GoVersion       string `json:"go_version"`
	HaAgentMetadata struct {
	} `json:"ha_agent_metadata"`
	HostTags []interface{} `json:"hostTags"`
	Hostinfo struct {
		BootTime             int    `json:"bootTime"`
		HostID               string `json:"hostId"`
		Hostname             string `json:"hostname"`
		KernelArch           string `json:"kernelArch"`
		KernelVersion        string `json:"kernelVersion"`
		Os                   string `json:"os"`
		Platform             string `json:"platform"`
		PlatformFamily       string `json:"platformFamily"`
		PlatformVersion      string `json:"platformVersion"`
		Procs                int    `json:"procs"`
		Uptime               int    `json:"uptime"`
		VirtualizationRole   string `json:"virtualizationRole"`
		VirtualizationSystem string `json:"virtualizationSystem"`
	} `json:"hostinfo"`
	HostnameStats struct {
		Errors struct {
			Aws       string `json:"aws"`
			Azure     string `json:"azure"`
			Container string `json:"container"`
			Fargate   string `json:"fargate"`
			Fqdn      string `json:"fqdn"`
			Gce       string `json:"gce"`
		} `json:"errors"`
		Provider string `json:"provider"`
	} `json:"hostnameStats"`
	Inventories struct {
	} `json:"inventories"`
	LogsStats struct {
		IsRunning        bool        `json:"is_running"`
		Endpoints        interface{} `json:"endpoints"`
		Metrics          interface{} `json:"metrics"`
		ProcessFileStats interface{} `json:"process_file_stats"`
		Integrations     interface{} `json:"integrations"`
		Tailers          interface{} `json:"tailers"`
		Errors           interface{} `json:"errors"`
		Warnings         interface{} `json:"warnings"`
		UseHTTP          bool        `json:"use_http"`
	} `json:"logsStats"`
	Message  string `json:"message"`
	Metadata struct {
		AgentFlavor string `json:"agent-flavor"`
		FipsMode    bool   `json:"fips_mode"`
		HostTags    struct {
			System []interface{} `json:"system"`
		} `json:"host-tags"`
		InstallMethod struct {
			InstallerVersion string `json:"installer_version"`
			Tool             string `json:"tool"`
			ToolVersion      string `json:"tool_version"`
		} `json:"install-method"`
		Logs struct {
			AutoMultiLineDetectionEnabled bool   `json:"auto_multi_line_detection_enabled"`
			Transport                     string `json:"transport"`
		} `json:"logs"`
		Meta struct {
			Ec2Hostname               string        `json:"ec2-hostname"`
			HostAliases               []interface{} `json:"host_aliases"`
			Hostname                  string        `json:"hostname"`
			HostnameResolutionVersion int           `json:"hostname-resolution-version"`
			InstanceID                string        `json:"instance-id"`
			SocketFqdn                string        `json:"socket-fqdn"`
			SocketHostname            string        `json:"socket-hostname"`
			Timezones                 []string      `json:"timezones"`
		} `json:"meta"`
		Network interface{} `json:"network"`
		Os      string      `json:"os"`
		Otlp    struct {
			Enabled bool `json:"enabled"`
		} `json:"otlp"`
		ProxyInfo struct {
			NoProxyNonexactMatch              bool `json:"no-proxy-nonexact-match"`
			NoProxyNonexactMatchExplicitlySet bool `json:"no-proxy-nonexact-match-explicitly-set"`
			ProxyBehaviorChanged              bool `json:"proxy-behavior-changed"`
		} `json:"proxy-info"`
		Python      string `json:"python"`
		SystemStats struct {
			CPUCores  int      `json:"cpuCores"`
			FbsdV     []string `json:"fbsdV"`
			MacV      []string `json:"macV"`
			Machine   string   `json:"machine"`
			NixV      []string `json:"nixV"`
			Platform  string   `json:"platform"`
			Processor string   `json:"processor"`
			PythonV   string   `json:"pythonV"`
			WinV      []string `json:"winV"`
		} `json:"systemStats"`
	} `json:"metadata"`
	NtpOffset float64 `json:"ntpOffset"`
	OtelAgent struct {
		// Error state (when DDOT is disabled or not running)
		Error string `json:"error,omitempty"`
		URL   string `json:"url,omitempty"`

		// Success state (when DDOT is running)
		AgentVersion     string `json:"agentVersion,omitempty"`
		CollectorVersion string `json:"collectorVersion,omitempty"`
	} `json:"otelAgent"`
	Otlp struct {
		OtlpCollectorStatus    string `json:"otlpCollectorStatus"`
		OtlpCollectorStatusErr string `json:"otlpCollectorStatusErr"`
		OtlpStatus             bool   `json:"otlpStatus"`
	} `json:"otlp"`
	Pid                int `json:"pid"`
	ProcessAgentStatus struct {
		Error string `json:"error"`
	} `json:"processAgentStatus"`
	ProcessComponentStatus struct {
		Core struct {
			BuildArch string `json:"build_arch"`
			Config    struct {
				LogLevel string `json:"log_level"`
			} `json:"config"`
			GoVersion string `json:"go_version"`
			Metadata  struct {
				AgentFlavor string `json:"agent-flavor"`
				FipsMode    bool   `json:"fips_mode"`
				HostTags    struct {
					System []interface{} `json:"system"`
				} `json:"host-tags"`
				InstallMethod struct {
					InstallerVersion string `json:"installer_version"`
					Tool             string `json:"tool"`
					ToolVersion      string `json:"tool_version"`
				} `json:"install-method"`
				Logs struct {
					AutoMultiLineDetectionEnabled bool   `json:"auto_multi_line_detection_enabled"`
					Transport                     string `json:"transport"`
				} `json:"logs"`
				Meta struct {
					Ec2Hostname               string        `json:"ec2-hostname"`
					HostAliases               []interface{} `json:"host_aliases"`
					Hostname                  string        `json:"hostname"`
					HostnameResolutionVersion int           `json:"hostname-resolution-version"`
					InstanceID                string        `json:"instance-id"`
					SocketFqdn                string        `json:"socket-fqdn"`
					SocketHostname            string        `json:"socket-hostname"`
					Timezones                 []string      `json:"timezones"`
				} `json:"meta"`
				Network interface{} `json:"network"`
				Os      string      `json:"os"`
				Otlp    struct {
					Enabled bool `json:"enabled"`
				} `json:"otlp"`
				ProxyInfo struct {
					NoProxyNonexactMatch              bool `json:"no-proxy-nonexact-match"`
					NoProxyNonexactMatchExplicitlySet bool `json:"no-proxy-nonexact-match-explicitly-set"`
					ProxyBehaviorChanged              bool `json:"proxy-behavior-changed"`
				} `json:"proxy-info"`
				Python      string `json:"python"`
				SystemStats struct {
					CPUCores  int      `json:"cpuCores"`
					FbsdV     []string `json:"fbsdV"`
					MacV      []string `json:"macV"`
					Machine   string   `json:"machine"`
					NixV      []string `json:"nixV"`
					Platform  string   `json:"platform"`
					Processor string   `json:"processor"`
					PythonV   string   `json:"pythonV"`
					WinV      []string `json:"winV"`
				} `json:"systemStats"`
			} `json:"metadata"`
			Version string `json:"version"`
		} `json:"core"`
		Date    int64 `json:"date"`
		Expvars struct {
			ProcessAgent struct {
				ConnectionsQueueBytes int           `json:"connections_queue_bytes"`
				ConnectionsQueueSize  int           `json:"connections_queue_size"`
				ContainerCount        int           `json:"container_count"`
				ContainerID           string        `json:"container_id"`
				DockerSocket          string        `json:"docker_socket"`
				DropCheckPayloads     []interface{} `json:"drop_check_payloads"`
				EnabledChecks         []string      `json:"enabled_checks"`
				Endpoints             struct {
					HTTPSProcessDatadoghqCom []string `json:"https://process.datadoghq.com."`
				} `json:"endpoints"`
				EventQueueBytes          int    `json:"event_queue_bytes"`
				EventQueueSize           int    `json:"event_queue_size"`
				LanguageDetectionEnabled bool   `json:"language_detection_enabled"`
				LastCollectTime          string `json:"last_collect_time"`
				LogFile                  string `json:"log_file"`
				Memstats                 struct {
					Alloc int `json:"alloc"`
				} `json:"memstats"`
				Pid                             int    `json:"pid"`
				ProcessCount                    int    `json:"process_count"`
				ProcessQueueBytes               int    `json:"process_queue_bytes"`
				ProcessQueueSize                int    `json:"process_queue_size"`
				ProxyURL                        string `json:"proxy_url"`
				RtprocessQueueBytes             int    `json:"rtprocess_queue_bytes"`
				RtprocessQueueSize              int    `json:"rtprocess_queue_size"`
				SubmissionErrorCount            int    `json:"submission_error_count"`
				SystemProbeProcessModuleEnabled bool   `json:"system_probe_process_module_enabled"`
				Uptime                          int    `json:"uptime"`
				UptimeNano                      int64  `json:"uptime_nano"`
				Version                         struct {
					BuildDate string `json:"BuildDate"`
					GitBranch string `json:"GitBranch"`
					GitCommit string `json:"GitCommit"`
					GoVersion string `json:"GoVersion"`
					Version   string `json:"Version"`
				} `json:"version"`
				WorkloadmetaExtractorCacheSize    int `json:"workloadmeta_extractor_cache_size"`
				WorkloadmetaExtractorDiffsDropped int `json:"workloadmeta_extractor_diffs_dropped"`
				WorkloadmetaExtractorStaleDiffs   int `json:"workloadmeta_extractor_stale_diffs"`
			} `json:"process_agent"`
		} `json:"expvars"`
	} `json:"processComponentStatus"`
	PyLoaderStats struct {
		ConfigureErrors struct {
		} `json:"ConfigureErrors"`
		Py3Warnings struct {
		} `json:"Py3Warnings"`
	} `json:"pyLoaderStats"`
	PythonInit struct {
		Errors []interface{} `json:"Errors"`
	} `json:"pythonInit"`
	PythonVersion       string `json:"python_version"`
	RemoteConfigStartup struct {
		StartupFailureReason string `json:"startupFailureReason"`
	} `json:"remoteConfigStartup"`
	RemoteConfiguration struct {
		APIKeyScoped string `json:"apiKeyScoped"`
		LastError    string `json:"lastError"`
		OrgEnabled   string `json:"orgEnabled"`
	} `json:"remoteConfiguration"`
	RunnerStats struct {
		Checks struct {
		} `json:"Checks"`
		Running struct {
		} `json:"Running"`
		RunningChecks int `json:"RunningChecks"`
		Runs          int `json:"Runs"`
		Workers       struct {
			Count int `json:"Count"`
		} `json:"Workers"`
	} `json:"runnerStats"`
	SnmpProfiles struct {
	} `json:"snmpProfiles"`
	SsiStatus struct {
		Modes struct {
			Docker bool `json:"docker"`
			Host   bool `json:"host"`
		} `json:"modes"`
		Status string `json:"status"`
	} `json:"ssiStatus"`
	TimeNano int64  `json:"time_nano"`
	Version  string `json:"version"`
}
