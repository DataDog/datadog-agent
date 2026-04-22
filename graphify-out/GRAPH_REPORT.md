# Graph Report - cmd/otel-agent + comp/otelcol  (2026-04-22)

## Corpus Check
- 178 files · ~82,811 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 1063 nodes · 2156 edges · 28 communities detected
- Extraction: 57% EXTRACTED · 43% INFERRED · 0% AMBIGUOUS · INFERRED: 934 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Community Hubs (Navigation)
- [[_COMMUNITY_APM Stats & Pipeline Tests|APM Stats & Pipeline Tests]]
- [[_COMMUNITY_Collector Pipeline Core|Collector Pipeline Core]]
- [[_COMMUNITY_Agent Config & Exporter Wiring|Agent Config & Exporter Wiring]]
- [[_COMMUNITY_Agent Run Command & Bundle|Agent Run Command & Bundle]]
- [[_COMMUNITY_Logs & Serializer Export|Logs & Serializer Export]]
- [[_COMMUNITY_Datadog Exporter & Traces|Datadog Exporter & Traces]]
- [[_COMMUNITY_Flare Archive & Config Store|Flare Archive & Config Store]]
- [[_COMMUNITY_Serializer Consumer & Config Check|Serializer Consumer & Config Check]]
- [[_COMMUNITY_Metrics Client & Subcommands|Metrics Client & Subcommands]]
- [[_COMMUNITY_Flare Extension Server & Profiling|Flare Extension Server & Profiling]]
- [[_COMMUNITY_Converter Autoconfigure|Converter Autoconfigure]]
- [[_COMMUNITY_Converter Design & Features|Converter Design & Features]]
- [[_COMMUNITY_Agent Entry Point & CLI|Agent Entry Point & CLI]]
- [[_COMMUNITY_InfraAttributes Processor|InfraAttributes Processor]]
- [[_COMMUNITY_Extension Configs|Extension Configs]]
- [[_COMMUNITY_Logs Agent Exporter|Logs Agent Exporter]]
- [[_COMMUNITY_Logs Agent Component|Logs Agent Component]]
- [[_COMMUNITY_Pipeline & Flare Filler|Pipeline & Flare Filler]]
- [[_COMMUNITY_Collector Component Factory|Collector Component Factory]]
- [[_COMMUNITY_Flare Diagram Components|Flare Diagram Components]]
- [[_COMMUNITY_Collector Consumer|Collector Consumer]]
- [[_COMMUNITY_DogTel Extension Metrics|DogTel Extension Metrics]]
- [[_COMMUNITY_Windows Control Service|Windows Control Service]]
- [[_COMMUNITY_Flare Extension Types|Flare Extension Types]]
- [[_COMMUNITY_OTLP Config Check|OTLP Config Check]]
- [[_COMMUNITY_Tagger Server Wrapper|Tagger Server Wrapper]]
- [[_COMMUNITY_InfraAttributes Config|InfraAttributes Config]]
- [[_COMMUNITY_Collector Status Type|Collector Status Type]]

## God Nodes (most connected - your core abstractions)
1. `NewConfigComponent()` - 62 edges
2. `ConfigTestSuite` - 49 edges
3. `createDefaultConfig()` - 44 edges
4. `NewFactory()` - 33 edges
5. `Module()` - 27 edges
6. `NewFactoryForAgent()` - 26 edges
7. `StatsdClientWrapper` - 21 edges
8. `metricsClient` - 21 edges
9. `runOTelAgentCommand()` - 18 edges
10. `newMetrics()` - 17 edges

## Surprising Connections (you probably didn't know these)
- `testResourceMetrics()` --calls--> `newMetrics()`  [INFERRED]
  /Users/stanley.liu/go/src/github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor/metrics_test.go → /Users/stanley.liu/go/src/github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter/exporter_test.go
- `infraattributes Processor (DDOT pipeline)` --semantically_similar_to--> `Infra Attributes Processor`  [INFERRED] [semantically similar]
  comp/otelcol/README.md → comp/otelcol/otlp/components/processor/infraattributesprocessor/README.md
- `TestNoURIsProvided()` --calls--> `NewConfigComponent()`  [INFERRED]
  /Users/stanley.liu/go/src/github.com/DataDog/datadog-agent/cmd/otel-agent/config/agent_config_test.go → /Users/stanley.liu/go/src/github.com/DataDog/datadog-agent/cmd/otel-agent/config/agent_config.go
- `TestLogsEnabledViaEnvironmentVariable()` --calls--> `NewConfigComponent()`  [INFERRED]
  /Users/stanley.liu/go/src/github.com/DataDog/datadog-agent/cmd/otel-agent/config/agent_config_test.go → /Users/stanley.liu/go/src/github.com/DataDog/datadog-agent/cmd/otel-agent/config/agent_config.go
- `TestLogsEnabledViaDatadogConfig()` --calls--> `NewConfigComponent()`  [INFERRED]
  /Users/stanley.liu/go/src/github.com/DataDog/datadog-agent/cmd/otel-agent/config/agent_config_test.go → /Users/stanley.liu/go/src/github.com/DataDog/datadog-agent/cmd/otel-agent/config/agent_config.go

## Hyperedges (group relationships)
- **Converter autoconfigures infraattributes, pprof, zpages, health_check, ddflare, prometheus into DDOT pipeline** — converter_readme_converter_component, infraattributes_readme_infra_attributes_processor, ddflare_readme_ddflare_extension, converter_readme_prometheus_feature, converter_readme_health_check_feature [EXTRACTED 0.95]
- **DDOT Collector pipeline: OTLP receiver → infraattributes processor → Datadog exporter/connector** — otelcol_readme_otlp_receiver, otelcol_readme_infraattributes_processor, otelcol_readme_datadog_exporter, otelcol_readme_datadog_connector [EXTRACTED 0.95]
- **dogtelextension standalone mode provides tagger, workload detection, and secrets resolution without core agent** — dogtelextension_readme_dogtel_extension, dogtelextension_readme_tagger_grpc_server, dogtelextension_readme_workload_detection, dogtelextension_readme_secrets_resolution [EXTRACTED 0.92]

## Communities

### Community 0 - "APM Stats & Pipeline Tests"
Cohesion: 0.05
Nodes (76): testAPMStats(), TestAPMStats_OSS(), TestAPMStats_OTelAgent(), testAPMStatsMetric(), setupShutdown(), TestConfigValidate_AutoFixConcurrentSync(), TestConfigValidate_AutoFixMaxMessageSize(), TestConfigValidate_KubeletTLSVerify_ExplicitFalse() (+68 more)

### Community 1 - "Collector Pipeline Core"
Cohesion: 0.05
Nodes (52): getBuildInfo(), getComponents(), NewPipeline(), NewPipelineFromAgentConfig(), recoverAndStoreError(), AssertFailedRun(), AssertSucessfulRun(), getTestPipelineConfig() (+44 more)

### Community 2 - "Agent Config & Exporter Wiring"
Cohesion: 0.06
Nodes (19): apiKeyItoa(), getDDExporterConfig(), getDogtelExtensionConfig(), getServiceConfig(), NewConfigComponent(), setSiteIfEmpty(), TestGetDogtelExtensionConfig_EmptyDogtelSection(), TestGetDogtelExtensionConfig_EnableMetadataCollectionFalse() (+11 more)

### Community 3 - "Agent Run Command & Bundle"
Cohesion: 0.04
Nodes (31): Bundle(), buildConfigURIs(), commonAgentFxOptions(), connectedAgentFxOptions(), ForwarderBundle(), runOTelAgentCommand(), standaloneAgentFxOptions(), TestFxRun_Disabled() (+23 more)

### Community 4 - "Logs & Serializer Export"
Cohesion: 0.05
Nodes (51): spanIDToHexOrEmptyString(), spanIDToUint64(), TestLogsExporter(), traceIDToHexOrEmptyString(), traceIDToUint64(), TestManifestCacheTTL(), InitSerializer(), setupForwarder() (+43 more)

### Community 5 - "Datadog Exporter & Traces"
Cohesion: 0.06
Nodes (30): NewComponent(), factory, mockLogsAgentPipeline, mockProvider, testComponent, traceExporter, addEmbeddedCollectorConfigWarnings(), checkAndCastConfig() (+22 more)

### Community 6 - "Flare Archive & Config Store"
Cohesion: 0.06
Nodes (35): addFileToZip(), createFlareArchive(), components(), makeModulesMap(), addFactories(), confmapFromResolverSettings(), newConfigProviderSettings(), newConverterFactory() (+27 more)

### Community 7 - "Serializer Consumer & Config Check"
Cohesion: 0.06
Nodes (24): convertToStringConfMap(), hasSection(), IsConfigEnabled(), readConfigSection(), ReadConfigSection(), TestHasSectionEdgeCases(), TestIsEnabled(), TestIsEnabledConsistencyWithReadConfigSection() (+16 more)

### Community 8 - "Metrics Client & Subcommands"
Cohesion: 0.06
Nodes (13): InitializeMetricClient(), setupMetricClient(), TestCount(), TestGauge(), TestGaugeDataRace(), TestGaugeMultiple(), TestHistogram(), TestNoNilMeter() (+5 more)

### Community 9 - "Flare Extension Server & Profiling"
Cohesion: 0.07
Nodes (25): mapReplaceValue(), newEnvConfMap(), newEnvToUUIDProvider(), sliceReplaceIfExists(), convertMapKeyAnyToStringAny(), mapToYAML(), newEnvConfMapFromYAML(), TestEnvConfMap_useEnvVarNames() (+17 more)

### Community 10 - "Converter Autoconfigure"
Cohesion: 0.08
Nodes (34): addComponentToConfig(), addComponentToPipeline(), componentName(), findComps(), newConverter(), NewConverterForAgent(), NewFactory(), filterLogsBySubstring() (+26 more)

### Community 11 - "Converter Design & Features"
Cohesion: 0.05
Nodes (46): Converter API Key and Site Auto-fetch Logic, Converter Autoconfigure Logic, Converter Component, Converter datadog OSS Extension Feature, Converter ddflare Extension Feature, Converter health_check Extension Feature, Converter infraattributes Feature, Rationale: Opting out of Converter loses flare, health metrics, infra tagging (+38 more)

### Community 12 - "Agent Entry Point & CLI"
Cohesion: 0.07
Nodes (34): collectOTelData(), createOTelFlare(), discoverDebugSources(), extractExtensionType(), extractHealthCheckEndpoint(), extractPprofEndpoint(), extractZpagesEndpoint(), flags() (+26 more)

### Community 13 - "InfraAttributes Processor"
Cohesion: 0.06
Nodes (22): entityIDsFromAttributes(), newInfraTagsProcessor(), originInfoFromAttributes(), splitTag(), data, factory, infraAttributesLogProcessor, infraAttributesMetricProcessor (+14 more)

### Community 14 - "Extension Configs"
Cohesion: 0.06
Nodes (21): healthExtractEndpoint(), pprofExtractEndpoint(), regularStringEndpointExtractor(), getTestConfig(), TestUnmarshal(), TestValidate(), zPagesExtractEndpoint(), Config (+13 more)

### Community 15 - "Logs Agent Exporter"
Cohesion: 0.09
Nodes (16): newDefaultConfig(), newDefaultConfigForAgent(), NewExporter(), translatorFromConfig(), NewPusher(), getLogsScope(), NewExporter(), NewExporterWithGatewayUsage() (+8 more)

### Community 16 - "Logs Agent Component"
Cohesion: 0.13
Nodes (10): buildEndpoints(), NewLogsAgent(), NewLogsAgentComponent(), createAgent(), TestAgentTestSuite(), TestBuildEndpoints(), Agent, AgentTestSuite (+2 more)

### Community 17 - "Pipeline & Flare Filler"
Cohesion: 0.11
Nodes (7): createFakeOTelExtensionHTTPServer(), TestOTelExtFlareBuilder(), toJSON(), collectorImpl, noopImpl, Provides, Requires

### Community 18 - "Collector Component Factory"
Cohesion: 0.12
Nodes (11): addFactories(), NewComponent(), NewComponentNoAgent(), newConfigProviderSettings(), setupShutdown(), collectorcontribImpl, collectorImpl, converterFactory (+3 more)

### Community 19 - "Flare Diagram Components"
Cohesion: 0.21
Nodes (15): Core Agent, DD App, DD Flare Extension, DD/Zendesk, Debug Exporters, Flare Component, HealthCheck, Inventory Meta Provider (+7 more)

### Community 20 - "Collector Consumer"
Cohesion: 0.32
Nodes (3): exporterDefaultMetrics(), tagsFromBuildInfo(), collectorConsumer

### Community 21 - "DogTel Extension Metrics"
Cohesion: 0.36
Nodes (6): CreateLivenessSerie(), TagsFromBuildInfo(), TestCreateLivenessSerie(), TestCreateLivenessSerie_EmptyTags(), TestCreateLivenessSerie_TimestampConversion(), TestTagsFromBuildInfo()

### Community 22 - "Windows Control Service"
Cohesion: 0.47
Nodes (4): Commands(), RestartService(), StartService(), StopService()

### Community 23 - "Flare Extension Types"
Cohesion: 0.33
Nodes (5): BuildInfoResponse, ConfigResponse, DebugSourceResponse, OTelFlareSource, Response

### Community 24 - "OTLP Config Check"
Cohesion: 0.4
Nodes (1): Pipeline

### Community 25 - "Tagger Server Wrapper"
Cohesion: 0.5
Nodes (1): taggerServerWrapper

### Community 26 - "InfraAttributes Config"
Cohesion: 0.67
Nodes (1): Config

### Community 28 - "Collector Status Type"
Cohesion: 1.0
Nodes (1): CollectorStatus

## Knowledge Gaps
- **73 isolated node(s):** `logLevel`, `dependencies`, `cliParams`, `cliParams`, `ProfilerOptions` (+68 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **Thin community `OTLP Config Check`** (5 nodes): `configcheck_no_otlp.go`, `IsDisplayed()`, `IsEnabled()`, `Pipeline`, `.Stop()`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `Tagger Server Wrapper`** (4 nodes): `tagger_server.go`, `taggerServerWrapper`, `.TaggerFetchEntity()`, `.TaggerStreamEntities()`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `InfraAttributes Config`** (3 nodes): `config.go`, `Config`, `.Validate()`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.
- **Thin community `Collector Status Type`** (2 nodes): `collector_status.go`, `CollectorStatus`
  Too small to be a meaningful cluster - may be noise or needs more connections extracted.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `NewConfigComponent()` connect `Agent Config & Exporter Wiring` to `APM Stats & Pipeline Tests`, `Agent Run Command & Bundle`, `Logs & Serializer Export`, `Datadog Exporter & Traces`, `Metrics Client & Subcommands`, `Converter Autoconfigure`?**
  _High betweenness centrality (0.094) - this node is a cross-community bridge._
- **Why does `NewFactory()` connect `Datadog Exporter & Traces` to `APM Stats & Pipeline Tests`, `Collector Pipeline Core`, `Agent Config & Exporter Wiring`, `Logs & Serializer Export`, `Flare Archive & Config Store`, `Flare Extension Server & Profiling`, `Converter Autoconfigure`, `Agent Entry Point & CLI`, `InfraAttributes Processor`, `Collector Component Factory`?**
  _High betweenness centrality (0.080) - this node is a cross-community bridge._
- **Why does `Module()` connect `Agent Run Command & Bundle` to `APM Stats & Pipeline Tests`, `Logs & Serializer Export`, `Datadog Exporter & Traces`, `Agent Entry Point & CLI`, `InfraAttributes Processor`?**
  _High betweenness centrality (0.064) - this node is a cross-community bridge._
- **Are the 58 inferred relationships involving `NewConfigComponent()` (e.g. with `TestNoURIsProvided()` and `.TestAgentConfig()`) actually correct?**
  _`NewConfigComponent()` has 58 INFERRED edges - model-reasoned connections that need verification._
- **Are the 41 inferred relationships involving `createDefaultConfig()` (e.g. with `getDDExporterConfig()` and `TestNewFactoryForAgent()`) actually correct?**
  _`createDefaultConfig()` has 41 INFERRED edges - model-reasoned connections that need verification._
- **Are the 26 inferred relationships involving `NewFactory()` (e.g. with `NewConfigComponent()` and `loadCustomerConfig()`) actually correct?**
  _`NewFactory()` has 26 INFERRED edges - model-reasoned connections that need verification._
- **Are the 18 inferred relationships involving `Module()` (e.g. with `MakeCommand()` and `runOTelAgentCommand()`) actually correct?**
  _`Module()` has 18 INFERRED edges - model-reasoned connections that need verification._