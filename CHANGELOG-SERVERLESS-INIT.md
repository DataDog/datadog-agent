# Serverless-init Changelog

<!-- This file tracks changes released in the `datadog/serverless-init` image.
     It is maintained manually, with sections newest first. To cut a release, generate
     the new section from the reno notes with
     `dda inv serverless-init.release-notes --prev <previous-tag> --sha <commit> --version <version>`,
     review it, and add it at the top. The release is built from the branch that adds
     it, and the PR is merged after the release.
     See the "Releasing Serverless-Init" runbook in Confluence (SLS space). -->

## 1.11.4
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.11.4)

* feat(serverless-init): Add env, service, and version tags to enhanced usage metrics [#58300](https://github.com/DataDog/datadog-agent/pull/58300)
* fix(serverless-init): wait for the final log batch when flushing serverless logs [#59170](https://github.com/DataDog/datadog-agent/pull/59170)
* fix(serverless-init): Flush pending APM stats on MicroVM /terminate [#58958](https://github.com/DataDog/datadog-agent/pull/58958)
* fix(serverless-init): Force out pending APM stats on MicroVM /suspend [#58960](https://github.com/DataDog/datadog-agent/pull/58960)
* [SVLS-9645] omit GCP tags for failed metadata lookups [#59145](https://github.com/DataDog/datadog-agent/pull/59145)

## 1.11.3
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.11.3)

* Remove `dogstatsd_packet_buffer_flush_timeout` override. serverless-init now uses the Agent default of 100ms, and the DogStatsD packet buffer flush timeout can be configured with `DD_DOGSTATSD_PACKET_BUFFER_FLUSH_TIMEOUT`.

## 1.11.2
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.11.2)

* serverless-init is now built with Go 1.26.10. This Go version rejects certificate chains containing compromised eMudhra intermediate CA public keys.

## 1.11.1
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.11.1)

* Dependency updates only

## 1.11.0
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.11.0)

* Improve MicroVM support and CPU metrics.
* Move to `distroless/base-nossl-debian13` for the regular and `distroless/static-debian13` for the alpine base images. This removes the unused `libssl` from the regular serverless-init images.

## 1.10.4
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.10.4)

* Add support for AWS Lambda MicroVMs

## 1.10.3
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.10.3)

* Dependency updates only

## 1.10.2
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.10.2)

* Dependency updates only

## 1.10.1
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.10.1)

* Dependency updates only

## 1.10.0
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.10.0)

* Simplify and consolidate metrics submission and metrics agent termination in serverless-init
* Remove `DD_TRACE_PROPAGATION_STYLE` environment variable override from in-container serverless-init.
* Respect existing `CORECLR_ENABLE_PROFILING`, `CORECLR_PROFILER`, `CORECLR_PROFILER_PATH`, and `DD_DOTNET_TRACER_HOME` .NET tracer environment variables when running in-container.

## 1.9.16
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.9.16)

* Remove the autodiscovery component since it is unused with serverless-init.

## 1.9.15
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.9.15)

* Dependency updates only

## 1.9.14
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.9.14)

* Dependency updates only

## 1.9.13
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.9.13)

* Resolve configuration issue blocking LLMObservability + OpenFeature functionality.

## 1.9.12
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.9.12)

* Fix Azure Container App metric prefix to `azure.app_containerapps`.

## 1.9.11
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.9.11)

* Add back arm64 builds for supporting local development. Note that Serverless Init is only officially supported for Azure/GCP serverless workloads -- for other workloads, please use the main [Datadog Agent](https://hub.docker.com/repository/docker/datadog/agent/general).
* Improvements to trace stats for Cloud Run Jobs related to shutdowns.

## 1.9.10
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.9.10)

* Bump go to 1.25.9 in [#49024](https://github.com/DataDog/datadog-agent/pull/49024)
* Remove arm64 builds
* Fix alpine sidecar container entrypoint

## 1.9.9
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.9.9)

* Add Enhanced CPU Metrics for Serverless in [#47421](https://github.com/DataDog/datadog-agent/pull/47421)
* Added a Cloud Run Jobs usage metric in [#48454](https://github.com/DataDog/datadog-agent/pull/48454)
* Skip trace/metric init when DD_API_KEY is not set in [#48458](https://github.com/DataDog/datadog-agent/pull/48458)

## 1.9.8
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.9.8)

* Dependency updates only

## 1.9.7
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.9.7)

* Fix Cloud Run Jobs shutdown metric race condition in [#47827](https://github.com/DataDog/datadog-agent/pull/47827)
* Fix Azure Container App env var parsing in [#44414](https://github.com/DataDog/datadog-agent/pull/44414)
* Add guards for disabled OTLP agent in serverless-int

## 1.9.6
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.9.6)

* Improved trace stats tagging.

## 1.9.5
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.9.5)

* Agent built with Go 1.25.7

## 1.9.4
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.9.4)

* [SVLS-8129] Add functionname tag to Google Cloud Run function profiles [#45459](https://github.com/DataDog/datadog-agent/pull/45459)
* [SVLS-8160] Add serverless-identifying tag to Azure App Service Windows profiles [#45281](https://github.com/DataDog/datadog-agent/pull/45281)
* feat: [SVLS-8340] use proper tags for the cloud run job span [#45335](https://github.com/DataDog/datadog-agent/pull/45335)

## 1.9.3
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.9.3)

* feat: [SVLS-8237] fix cloud run job trace parenting [#44727](https://github.com/DataDog/datadog-agent/pull/44727)
* [SVLS-8160] Add serverless-identifying tags to Azure App Service and Azure Container App profiles [#44361](https://github.com/DataDog/datadog-agent/pull/44361)

## 1.9.2
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.9.2)

* Resolve missing Azure Container Apps `subscription_id` and Resource Group tags in [#44414](https://github.com/DataDog/datadog-agent/pull/44414)
* Isolate internal trace stats tag to traces only in [#43370](https://github.com/DataDog/datadog-agent/pull/43370)

## 1.9.1
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.9.1)

* Agent built with go 1.25.5

## 1.9.0
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.9.0)

* [Serverless] Remove Lambda Extension in [#42994](https://github.com/DataDog/datadog-agent/pull/42994)
* [Serverless] Change serverless-init log level & remove noisy logs in [#42657](https://github.com/DataDog/datadog-agent/pull/42657)
* Replace serverless logger with log wrapper in [#42496](https://github.com/DataDog/datadog-agent/pull/42496)
* [AGENTRUN-901] Update http://golang.org/x/... dependencies in [#43275](https://github.com/DataDog/datadog-agent/pull/43275)
* [Serverless-Init] Cloud Run Jobs Inferred Span in [#43371](https://github.com/DataDog/datadog-agent/pull/43371)
* [Cloud Run Jobs] Remove high cardinality tags from enhanced metrics in [#43837](https://github.com/DataDog/datadog-agent/pull/43837)
* [Cloud Run Jobs] Replace exit_code tag with error:true or error:false to reduce cardinality in [#43694](https://github.com/DataDog/datadog-agent/pull/43694)

## 1.8.2
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.8.2)

* Fix cloudRunPrefix from 'gpc.run' to 'gcp.run' in [#42416](https://github.com/DataDog/datadog-agent/pull/42416)
* [Serverless] Fix AAS Duplicated logs on restart in [#42244](https://github.com/DataDog/datadog-agent/pull/42244)

## 1.8.1
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.8.1)

* fix(serverless-init): add nil channel check for service check and event flushing in [#41539](https://github.com/DataDog/datadog-agent/pull/41539)
* [Serverless-Init] Improve Tags Performance in [#41582](https://github.com/DataDog/datadog-agent/pull/41582)

## 1.8.0
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.8.0)

* Fix trace stats, ensuring agent-side or tracer-side stats computation and disabling backend stats computation.
* Fix performance.

## 1.7.12
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.7.12)

* Add `exit_code` tag to Cloud Run Jobs shutdown metric in [#41250](https://github.com/DataDog/datadog-agent/pull/41250)
* Use `zstd` compression instead of `glib` for better logging performance in [#41353](https://github.com/DataDog/datadog-agent/pull/41353)

## 1.7.11
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.7.11)

* Fix AAS Profiling Billing in [#40371](https://github.com/DataDog/datadog-agent/pull/40371)
* Add a log file descriptor for AAS instance logging in [#40591](https://github.com/DataDog/datadog-agent/pull/40591)
* Update Debian to fix [CVE-2025-4802](https://avd.aquasec.com/nvd/2025/cve-2025-4802/) and [CVE-2025-8058](https://avd.aquasec.com/nvd/2025/cve-2025-8058/)

## 1.7.10
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.7.10)

* adds a new env var `DD_AAS_INSTANCE_LOG_FILE_DESCRIPTOR` to enable single file logging when `DD_AAS_INSTANCE_LOGGING_ENABLED` is set to true in [#40591](https://github.com/DataDog/datadog-agent/pull/40591)

## 1.7.9
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.7.9)

* fix(serverless-init): set serverless log hostname to empty string in [#40074](https://github.com/DataDog/datadog-agent/pull/40074)
* Cloud Run Jobs enhanced metrics in [39246](https://github.com/DataDog/datadog-agent/pull/39246)

## 1.7.8
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.7.8)

* vulnerability: upgrade go to 1.24.6 in [#782](https://github.com/DataDog/datadog-lambda-extension/pull/782)
* [Serverless-Init] Support Cloud Run Jobs with serverless-init in [#38737](https://github.com/DataDog/datadog-agent/pull/38737)

## 1.7.7
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.7.7)

* use GITHUB_TOKEN if available in Serverless-Init .NET install script in [#748](https://github.com/DataDog/datadog-lambda-extension/pull/748)

## 1.7.6
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.7.6)

* Fix Trace Tagging. `DD_TAGS` is applied as trace tags for backend indexing and processing.

## 1.7.5
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.7.5)

* Fix [CVE-2025-22874](https://github.com/advisories/GHSA-6f52-wpx2-hvf2)

## 1.7.4
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.7.4)

* Fix multiline stacktraces being split up into multiple logs (when serverless-init is installed in-process) in [#37551](https://github.com/DataDog/datadog-agent/pull/37551)
* Fix flushing error logs (for in-process serverless-init) in [#37710](https://github.com/DataDog/datadog-agent/pull/37710)

## 1.7.3
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.7.3)

* Identical to v1.7.1

## 1.7.2
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.7.2)

> WARNING: v1.7.2 can cause CPU spikes or crashes. Please use v1.7.3.

* Fix multiline stacktraces being split up into multiple logs (when serverless-init is installed in-process) in [#37551](https://github.com/DataDog/datadog-agent/pull/37551)

## 1.7.1
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.7.1)

* Fix [CVE-2023-4039](https://ubuntu.com/security/CVE-2023-4039) in [#1802](https://github.com/GoogleContainerTools/distroless/issues/1802)

## 1.7.0
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.7.0)

* Adds an additional option for sidecars in Azure App Services to enable per instance log tailing. [#36530](https://github.com/DataDog/datadog-agent/pull/36530)

## 1.6.1
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.6.1)

* Fix [CVE-2025-22871](https://avd.aquasec.com/nvd/2025/cve-2025-22871/) in [#642](https://github.com/DataDog/datadog-lambda-extension/pull/642)

## 1.6.0
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.6.0)

* updates `function_target` tag to `build_function_target` for GCP cloud run functions

## 1.5.9
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.5.9)

* Fix [CVE-2025-0395](https://github.com/advisories/GHSA-4xpw-6594-8f5m) in [#1768](https://github.com/GoogleContainerTools/distroless/pull/1768)

## 1.5.8
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.5.8)

* Update with a security patch for http-proxy in [#34923](https://github.com/DataDog/datadog-agent/pull/34923)

## 1.5.7
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.5.7)

* Update with a security patch for stdlib in [#547](https://github.com/DataDog/datadog-lambda-extension/pull/547)

## 1.5.6
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.5.6)

* Add /dd_tracer/node/node_modules to NODE_PATH for serverless-init in [#33589](https://github.com/DataDog/datadog-agent/pull/33589)

## 1.5.5
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.5.5)

* Exposes gcp resource_name tag to traces and logs.
* Refactors and simplifies tagging logic for gcp to reduce memory consumption

## 1.5.4
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.5.4)

* Fixes a segmentation fault when a dogstatsd port is already in use for certain serverless container environments.

## 1.5.3
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.5.3)

* Address [CVE-2024-45338](https://avd.aquasec.com/nvd/cve-2024-45338): Non-linear parsing of case-insensitive content in golang.org/x/net/html

## 1.5.2
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.5.2)

* Fixes a bug in the previous release in this PR [[ASCII-2567] propagates the tagger component when building the OTLP pipeline for serverless-init](https://github.com/DataDog/datadog-agent/pull/31576)
* updates `gcr` namespace tagging

## 1.5.1
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.5.1)

* Address CVE-2024-5535: Bumping packages to latest stable versions in [#1710](https://github.com/GoogleContainerTools/distroless/pull/1710)

## 1.5.0
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.5.0)

* Fix race condition setting global tags in Azure Container Apps in [#30743](https://github.com/DataDog/datadog-agent/pull/30743)
* Add Back Tags to Spans Sent from Serverless-Init in [#30813](https://github.com/DataDog/datadog-agent/pull/30813)
* [SVLS-5917] Add Azure Containerapp namespace for v2 billing in [#30783](https://github.com/DataDog/datadog-agent/pull/30783)
* [SVLS-5506] Update cloud run tags to also account for Cloud Run Functions and Namespace in [#30722](https://github.com/DataDog/datadog-agent/pull/30722)

## 1.4.0
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.4.0)

* Add namespaced azure container app tags to spans and profiles in [#30637](https://github.com/DataDog/datadog-agent/pull/30637)

## 1.3.0
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.3.0)

* [SVLS-5713] Correctly set log source in sidecar mode in [#30070](https://github.com/DataDog/datadog-agent/pull/30070)
* [Serverless] Update sidecar tagging for V2 billing in [#30463](https://github.com/DataDog/datadog-agent/pull/30463)
* Add Metadata to Profiles from Azure Container Apps in [#30428](https://github.com/DataDog/datadog-agent/pull/30428)
* Bump go version in alpine image to 1.23.2 in [#426](https://github.com/DataDog/datadog-lambda-extension/pull/426)

## 1.2.9

* Fixes Bug from 1.2.8

## 1.2.8
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.2.8)

Agent Version: 7.56.0

* bundle .NET tracer script with serverless-init [#366](https://github.com/DataDog/datadog-lambda-extension/pull/366)

## 1.2.7
[Agent Tag](https://github.com/DataDog/datadog-agent/releases/tag/serverless-init-1.2.7)

Agent Version: 7.55.3

* histogram_aggregates and histogram_percentiles added to serverless initialization [#28102](https://github.com/DataDog/datadog-agent/pull/28102)

## 1.2.6

* Updates serverless-init to default DD_LOGS_ENABLED to false
* Updates sidecar mode to default DD_LOGS_ENABLED to true

## 1.2.5

* Adds sidecar mode for running in a sidecar
* Fixes missing tags in some Azure environments
* Propagate exit codes when an instrumented application errors

## 1.2.4

* Reverts to 1.2.2 due to a bug in 1.2.3

## 1.2.3

**There is a known issue with this version. We recommend using another version.**

* Adds sidecar mode for running in a sidecar
* Fixes missing tags in some Azure environments

## 1.2.2

* Updates dependencies

## 1.2.1

* Compute trace stats for serverless-init
* Fix incorrect `_dd.origin` tag for Azure + GCP serverless services

## 1.1.4

* Fixes a vulnerability in opentelemetry.

## 1.1.3

* Fixes an issue with incorrect metric bucketing that would sometimes lead to wrong values.
* Fixes an issue with log line parsing in the agent that would lead to duplicated, truncated, or missing logs sometimes.

## 1.1.2

* Fixes propagation of OS signals

## 1.1.1

* Fixes trace agent logs not being shown despite DD_LOG_LEVEL being set to debug
* Removes some debug messages

## 1.1.0

* Fix logs splitting on .NET apps.

## 1.0.0

* Adds support for automatically setting tracer environment variables.

## beta9

* Fixes a log buffering issue that caused log lines to be split up.
* Adds support for DD_AZURE_RESOURCE_GROUP and DD_AZURE_SUBSCRIPTION_ID.

## beta8

* Adds support for OTLP.
* Fixes an issue where DD_TAG and DD_EXTRA_TAG were ignored.

## beta7

* Fixes a regression in beta6 that had a small chance to cause application panics on startup.
* Fixes an issue where custom metrics were not sent to Datadog.
* Fixes an issue where Azure Container Apps logs were not correctly tagged with `service`.
* Fixes an issue where Azure Container Apps services would sometimes send blank logs to Datadog.

## beta6

* Adds support for DD_PROXY_HTTP and DD_PROXY_HTTPS

## beta5

* Adds support for Azure Container Apps.
* Removes top level span (gcp.cloudrun) from Google Cloud Run.
