1769815648022758079 [Info] config lib used: viper
=== RUN   TestServiceExtractorConcurrentAccess
    mock.go:28: 2026-01-30 15:27:28 PST | INFO | (comp/core/workloadmeta/impl/store.go:104 in start) | workloadmeta store initialized successfully
    mock.go:28: 2026-01-30 15:27:28 PST | INFO | (pkg/config/setup/config.go:2524 in loadCustom) | Starting to load the configuration
    mock.go:28: 2026-01-30 15:27:28 PST | DEBUG | (pkg/config/viperconfig/viper.go:124 in Set) | Updating setting 'system_probe_config.sysprobe_socket' for source 'agent-runtime' with the same value, skipping notification
    mock.go:28: 2026-01-30 15:27:28 PST | DEBUG | (pkg/config/viperconfig/viper.go:120 in Set) | Updating setting 'system_probe_config.allow_prebuilt_fallback' for source 'agent-runtime' with new value. notifying 0 listeners
    mock.go:28: 2026-01-30 15:27:28 PST | DEBUG | (pkg/config/viperconfig/viper.go:120 in Set) | Updating setting 'network_config.closed_channel_size' for source 'agent-runtime' with new value. notifying 0 listeners
    mock.go:28: 2026-01-30 15:27:28 PST | DEBUG | (pkg/config/viperconfig/viper.go:124 in Set) | Updating setting 'system_probe_config.max_tracked_connections' for source 'agent-runtime' with the same value, skipping notification
    mock.go:28: 2026-01-30 15:27:28 PST | DEBUG | (pkg/config/viperconfig/viper.go:120 in Set) | Updating setting 'system_probe_config.max_closed_connections_buffered' for source 'agent-runtime' with new value. notifying 0 listeners
    mock.go:28: 2026-01-30 15:27:28 PST | DEBUG | (pkg/config/viperconfig/viper.go:120 in Set) | Updating setting 'network_config.max_failed_connections_buffered' for source 'agent-runtime' with new value. notifying 0 listeners
    mock.go:28: 2026-01-30 15:27:28 PST | DEBUG | (pkg/config/viperconfig/viper.go:120 in Set) | Updating setting 'event_monitoring_config.network_process.max_processes_tracked' for source 'agent-runtime' with new value. notifying 0 listeners
    mock.go:28: 2026-01-30 15:27:28 PST | DEBUG | (pkg/config/viperconfig/viper.go:120 in Set) | Updating setting 'service_monitoring_config.max_concurrent_requests' for source 'agent-runtime' with new value. notifying 0 listeners
    mock.go:28: 2026-01-30 15:27:28 PST | DEBUG | (pkg/config/viperconfig/viper.go:124 in Set) | Updating setting 'service_monitoring_config.http.notification_threshold' for source 'agent-runtime' with the same value, skipping notification
    mock.go:28: 2026-01-30 15:27:28 PST | DEBUG | (pkg/config/viperconfig/viper.go:120 in Set) | Updating setting 'service_monitoring_config.disable_map_preallocation' for source 'agent-runtime' with new value. notifying 0 listeners
    mock.go:28: 2026-01-30 15:27:28 PST | DEBUG | (pkg/config/viperconfig/viper.go:120 in Set) | Updating setting 'system_probe_config.adjusted' for source 'agent-runtime' with new value. notifying 0 listeners
    mock.go:28: 2026-01-30 15:27:28 PST | DEBUG | (pkg/config/viperconfig/viper.go:120 in Set) | Updating setting 'discovery.enabled' for source 'agent-runtime' with new value. notifying 0 listeners
    mock.go:28: 2026-01-30 15:27:28 PST | DEBUG | (pkg/config/viperconfig/viper.go:120 in Set) | Updating setting 'system_probe_config.enabled' for source 'agent-runtime' with new value. notifying 0 listeners
    mock.go:28: 2026-01-30 15:27:28 PST | DEBUG | (pkg/util/cloudproviders/network/network.go:46 in func1) | GetNetworkID trying GCE
    mock.go:28: 2026-01-30 15:27:28 PST | DEBUG | (pkg/config/utils/miscellaneous.go:90 in IsCloudProviderEnabled) | cloud_provider_metadata is set to [aws gcp azure alibaba oracle ibm] in agent configuration, trying endpoints for GCP Cloud Provider
    mock.go:28: 2026-01-30 15:27:29 PST | DEBUG | (pkg/util/cloudproviders/network/network.go:52 in func1) | GetNetworkID trying EC2
    mock.go:28: 2026-01-30 15:27:29 PST | DEBUG | (pkg/config/utils/miscellaneous.go:90 in IsCloudProviderEnabled) | cloud_provider_metadata is set to [aws gcp azure alibaba oracle ibm] in agent configuration, trying endpoints for AWS Cloud Provider
    mock.go:28: 2026-01-30 15:27:29 PST | DEBUG | (pkg/network/sender/network.go:32 in retryGetNetworkID) | failed to fetch network ID (attempt 1/4): could not detect network ID: EC2: GetNetworkID failed to get mac addresses: could not fetch token from IMDSv2: Put "http://169.254.169.254/latest/api/token": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
    mock.go:28: 2026-01-30 15:27:29 PST | DEBUG | (pkg/util/cloudproviders/network/network.go:46 in func1) | GetNetworkID trying GCE
    mock.go:28: 2026-01-30 15:27:29 PST | DEBUG | (pkg/config/utils/miscellaneous.go:90 in IsCloudProviderEnabled) | cloud_provider_metadata is set to [aws gcp azure alibaba oracle ibm] in agent configuration, trying endpoints for GCP Cloud Provider
    mock.go:28: 2026-01-30 15:27:30 PST | DEBUG | (pkg/util/cloudproviders/network/network.go:52 in func1) | GetNetworkID trying EC2
    mock.go:28: 2026-01-30 15:27:30 PST | DEBUG | (pkg/config/utils/miscellaneous.go:90 in IsCloudProviderEnabled) | cloud_provider_metadata is set to [aws gcp azure alibaba oracle ibm] in agent configuration, trying endpoints for AWS Cloud Provider
    mock.go:28: 2026-01-30 15:27:31 PST | DEBUG | (pkg/network/sender/network.go:32 in retryGetNetworkID) | failed to fetch network ID (attempt 2/4): could not detect network ID: EC2: GetNetworkID failed to get mac addresses: could not fetch token from IMDSv2: Put "http://169.254.169.254/latest/api/token": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
    mock.go:28: 2026-01-30 15:27:31 PST | DEBUG | (pkg/util/cloudproviders/network/network.go:46 in func1) | GetNetworkID trying GCE
    mock.go:28: 2026-01-30 15:27:31 PST | DEBUG | (pkg/config/utils/miscellaneous.go:90 in IsCloudProviderEnabled) | cloud_provider_metadata is set to [aws gcp azure alibaba oracle ibm] in agent configuration, trying endpoints for GCP Cloud Provider
    mock.go:28: 2026-01-30 15:27:32 PST | DEBUG | (pkg/util/cloudproviders/network/network.go:52 in func1) | GetNetworkID trying EC2
    mock.go:28: 2026-01-30 15:27:32 PST | DEBUG | (pkg/config/utils/miscellaneous.go:90 in IsCloudProviderEnabled) | cloud_provider_metadata is set to [aws gcp azure alibaba oracle ibm] in agent configuration, trying endpoints for AWS Cloud Provider
    mock.go:28: 2026-01-30 15:27:32 PST | DEBUG | (pkg/network/sender/network.go:32 in retryGetNetworkID) | failed to fetch network ID (attempt 3/4): could not detect network ID: EC2: GetNetworkID failed to get mac addresses: could not fetch token from IMDSv2: Put "http://169.254.169.254/latest/api/token": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
    mock.go:28: 2026-01-30 15:27:33 PST | DEBUG | (pkg/util/cloudproviders/network/network.go:46 in func1) | GetNetworkID trying GCE
    mock.go:28: 2026-01-30 15:27:33 PST | DEBUG | (pkg/config/utils/miscellaneous.go:90 in IsCloudProviderEnabled) | cloud_provider_metadata is set to [aws gcp azure alibaba oracle ibm] in agent configuration, trying endpoints for GCP Cloud Provider
    mock.go:28: 2026-01-30 15:27:34 PST | DEBUG | (pkg/util/cloudproviders/network/network.go:52 in func1) | GetNetworkID trying EC2
    mock.go:28: 2026-01-30 15:27:34 PST | DEBUG | (pkg/config/utils/miscellaneous.go:90 in IsCloudProviderEnabled) | cloud_provider_metadata is set to [aws gcp azure alibaba oracle ibm] in agent configuration, trying endpoints for AWS Cloud Provider
    mock.go:28: 2026-01-30 15:27:34 PST | DEBUG | (pkg/network/sender/network.go:32 in retryGetNetworkID) | failed to fetch network ID (attempt 4/4): could not detect network ID: EC2: GetNetworkID failed to get mac addresses: could not fetch token from IMDSv2: Put "http://169.254.169.254/latest/api/token": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
    mock.go:28: 2026-01-30 15:27:34 PST | INFO | (pkg/network/sender/sender_linux.go:92 in New) | network ID not detected: failed to get network ID after 4 attempts: could not detect network ID: EC2: GetNetworkID failed to get mac addresses: could not fetch token from IMDSv2: Put "http://169.254.169.254/latest/api/token": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
    mock.go:28: 2026-01-30 15:27:34 PST | DEBUG | (pkg/hosttags/host_tag_provider.go:43 in newHostTagProviderWithClock) | Adding host tags to metrics for 30m0s
    mock.go:28: 2026-01-30 15:27:34 PST | DEBUG | (pkg/util/hostname/providers.go:211 in getHostname) | trying to get hostname from 'configuration' provider
    mock.go:28: 2026-01-30 15:27:34 PST | DEBUG | (pkg/util/hostname/providers.go:218 in getHostname) | unable to get the hostname from 'configuration' provider: hostname is empty
    mock.go:28: 2026-01-30 15:27:34 PST | DEBUG | (pkg/util/hostname/providers.go:211 in getHostname) | trying to get hostname from 'hostnameFile' provider
    mock.go:28: 2026-01-30 15:27:34 PST | DEBUG | (pkg/util/hostname/providers.go:218 in getHostname) | unable to get the hostname from 'hostnameFile' provider: 'hostname_file' configuration is not enabled
    mock.go:28: 2026-01-30 15:27:34 PST | DEBUG | (pkg/util/hostname/providers.go:211 in getHostname) | trying to get hostname from 'fargate' provider
    mock.go:28: 2026-01-30 15:27:34 PST | DEBUG | (pkg/util/hostname/providers.go:218 in getHostname) | unable to get the hostname from 'fargate' provider: agent is not running in sidecar mode
    mock.go:28: 2026-01-30 15:27:34 PST | DEBUG | (pkg/util/hostname/providers.go:211 in getHostname) | trying to get hostname from 'gce' provider
    mock.go:28: 2026-01-30 15:27:34 PST | DEBUG | (pkg/config/utils/miscellaneous.go:90 in IsCloudProviderEnabled) | cloud_provider_metadata is set to [aws gcp azure alibaba oracle ibm] in agent configuration, trying endpoints for GCP Cloud Provider
    mock.go:28: 2026-01-30 15:27:35 PST | DEBUG | (pkg/util/hostname/providers.go:218 in getHostname) | unable to get the hostname from 'gce' provider: unable to retrieve hostname from GCE: GCE metadata API error: Get "http://169.254.169.254/computeMetadata/v1/instance/hostname": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
    mock.go:28: 2026-01-30 15:27:35 PST | DEBUG | (pkg/util/hostname/providers.go:211 in getHostname) | trying to get hostname from 'azure' provider
    mock.go:28: 2026-01-30 15:27:35 PST | DEBUG | (pkg/util/hostname/providers.go:218 in getHostname) | unable to get the hostname from 'azure' provider: azure_hostname_style is set to 'os'
    mock.go:28: 2026-01-30 15:27:35 PST | DEBUG | (pkg/util/hostname/providers.go:211 in getHostname) | trying to get hostname from 'fqdn' provider
    mock.go:28: 2026-01-30 15:27:35 PST | DEBUG | (pkg/util/hostname/providers.go:218 in getHostname) | unable to get the hostname from 'fqdn' provider: 'hostname_fqdn' configuration is not enabled
    mock.go:28: 2026-01-30 15:27:35 PST | DEBUG | (pkg/util/hostname/providers.go:211 in getHostname) | trying to get hostname from 'container' provider
    mock.go:28: 2026-01-30 15:27:35 PST | DEBUG | (pkg/util/hostname/providers.go:218 in getHostname) | unable to get the hostname from 'container' provider: the agent is not containerized
    mock.go:28: 2026-01-30 15:27:35 PST | DEBUG | (pkg/util/hostname/providers.go:211 in getHostname) | trying to get hostname from 'os' provider
    mock.go:28: 2026-01-30 15:27:35 PST | DEBUG | (pkg/util/hostname/providers.go:222 in getHostname) | hostname provider 'os' successfully found hostname 'agent-dev-ubuntu-22'
    mock.go:28: 2026-01-30 15:27:35 PST | DEBUG | (pkg/util/hostname/providers.go:211 in getHostname) | trying to get hostname from 'aws' provider
    mock.go:28: 2026-01-30 15:27:35 PST | DEBUG | (pkg/util/hostname/common.go:141 in resolveEC2Hostname) | Detected a default EC2 hostname: false
    mock.go:28: 2026-01-30 15:27:35 PST | DEBUG | (pkg/util/hostname/common.go:142 in resolveEC2Hostname) | ec2_prioritize_instance_id_as_hostname is set to false
    mock.go:28: 2026-01-30 15:27:35 PST | DEBUG | (pkg/util/hostname/providers.go:218 in getHostname) | unable to get the hostname from 'aws' provider: not retrieving hostname from AWS: the host is not an ECS instance and other providers already retrieve non-default hostnames
    mock.go:28: 2026-01-30 15:27:35 PST | DEBUG | (pkg/config/utils/clusteragent.go:53 in GetClusterAgentEndpoint) | Identified service for the Datadog Cluster Agent: datadog-cluster-agent
    mock.go:28: 2026-01-30 15:27:36 PST | DEBUG | (pkg/util/clusteragent/clusteragent.go:116 in GetClusterAgentClient) | Cluster Agent init error: temporary failure in clusterAgentClient, will retry later: cannot get a cluster agent endpoint for kubernetes service DATADOG_CLUSTER_AGENT, env DATADOG_CLUSTER_AGENT_SERVICE_HOST is empty
    mock.go:28: 2026-01-30 15:27:36 PST | DEBUG | (pkg/config/utils/miscellaneous.go:90 in IsCloudProviderEnabled) | cloud_provider_metadata is set to [aws gcp azure alibaba oracle ibm] in agent configuration, trying endpoints for GCP Cloud Provider
    mock.go:28: 2026-01-30 15:27:36 PST | DEBUG | (pkg/config/utils/miscellaneous.go:90 in IsCloudProviderEnabled) | cloud_provider_metadata is set to [aws gcp azure alibaba oracle ibm] in agent configuration, trying endpoints for GCP Cloud Provider
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (comp/metadata/host/hostimpl/hosttags/tags.go:176 in Get) | No gce host tags, remaining attempts: 0, err: unable to get tags from gce and cache is empty: GCE metadata API error: Get "http://169.254.169.254/computeMetadata/v1/?recursive=true": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
    mock.go:28: 2026-01-30 15:27:37 PST | INFO | (comp/metadata/host/hostimpl/hosttags/tags.go:178 in Get) | Unable to get host tags from source: gce - using cached host tags
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/cgroups/pid_mapper.go:82 in getPidMapper) | Using cgroup.procs for pid mapping
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/cgroups/pid_mapper.go:82 in getPidMapper) | Using cgroup.procs for pid mapping
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/system/collector_linux.go:173 in newSystemCollector) | Chosen system collectors: &{Stats:{Collector:0xc0004b7180 Priority:0} Network:{Collector:0xc0004b7180 Priority:0} OpenFilesCount:{Collector:0xc0004b7180 Priority:0} PIDs:{Collector:0xc0004b7180 Priority:0} ContainerIDForPID:{Collector:0xc0004b7180 Priority:0} ContainerIDForInode:{Collector:0xc0004b7180 Priority:0} SelfContainerID:{Collector:0xc0004b7180 Priority:0} ContainerIDForPodUIDAndContName:{Collector:<nil> Priority:0}}
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerStatsGetter] and runtime: ecsmanagedinstances
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerNetworkStatsGetter] and runtime: ecsmanagedinstances
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerOpenFilesCountGetter] and runtime: ecsmanagedinstances
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerPIDsGetter] and runtime: ecsmanagedinstances
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerIDForPIDRetriever] and runtime: ecsmanagedinstances
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerIDForInodeRetriever] and runtime: ecsmanagedinstances
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.SelfContainerIDRetriever] and runtime: ecsmanagedinstances
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerStatsGetter] and runtime: cri-nonstandard
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerNetworkStatsGetter] and runtime: cri-nonstandard
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerOpenFilesCountGetter] and runtime: cri-nonstandard
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerPIDsGetter] and runtime: cri-nonstandard
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerIDForPIDRetriever] and runtime: cri-nonstandard
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerIDForInodeRetriever] and runtime: cri-nonstandard
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.SelfContainerIDRetriever] and runtime: cri-nonstandard
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerStatsGetter] and runtime: docker
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerNetworkStatsGetter] and runtime: docker
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerOpenFilesCountGetter] and runtime: docker
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerPIDsGetter] and runtime: docker
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerIDForPIDRetriever] and runtime: docker
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerIDForInodeRetriever] and runtime: docker
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.SelfContainerIDRetriever] and runtime: docker
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerStatsGetter] and runtime: containerd
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerNetworkStatsGetter] and runtime: containerd
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerOpenFilesCountGetter] and runtime: containerd
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerPIDsGetter] and runtime: containerd
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerIDForPIDRetriever] and runtime: containerd
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerIDForInodeRetriever] and runtime: containerd
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.SelfContainerIDRetriever] and runtime: containerd
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerStatsGetter] and runtime: cri-o
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerNetworkStatsGetter] and runtime: cri-o
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerOpenFilesCountGetter] and runtime: cri-o
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerPIDsGetter] and runtime: cri-o
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerIDForPIDRetriever] and runtime: cri-o
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerIDForInodeRetriever] and runtime: cri-o
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.SelfContainerIDRetriever] and runtime: cri-o
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerStatsGetter] and runtime: garden
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerNetworkStatsGetter] and runtime: garden
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerOpenFilesCountGetter] and runtime: garden
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerPIDsGetter] and runtime: garden
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerIDForPIDRetriever] and runtime: garden
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerIDForInodeRetriever] and runtime: garden
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.SelfContainerIDRetriever] and runtime: garden
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerStatsGetter] and runtime: podman
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerNetworkStatsGetter] and runtime: podman
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerOpenFilesCountGetter] and runtime: podman
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerPIDsGetter] and runtime: podman
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerIDForPIDRetriever] and runtime: podman
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerIDForInodeRetriever] and runtime: podman
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.SelfContainerIDRetriever] and runtime: podman
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerStatsGetter] and runtime: ecsfargate
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerNetworkStatsGetter] and runtime: ecsfargate
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerOpenFilesCountGetter] and runtime: ecsfargate
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerPIDsGetter] and runtime: ecsfargate
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerIDForPIDRetriever] and runtime: ecsfargate
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.ContainerIDForInodeRetriever] and runtime: ecsfargate
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/util/containers/metrics/provider/collector.go:35 in bestCollector) | Using collector id: system for type: provider.CollectorRef[github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider.SelfContainerIDRetriever] and runtime: ecsfargate
    mock.go:28: 2026-01-30 15:27:37 PST | INFO | (pkg/network/sender/sender_linux.go:240 in start) | direct sender started
    mock.go:28: 2026-01-30 15:27:37 PST | DEBUG | (pkg/config/viperconfig/viper.go:120 in Set) | Updating setting 'system_probe_config.process_service_inference.enabled' for source 'unknown' with new value. notifying 0 listeners
==================
WARNING: DATA RACE
Read at 0x00c001647b60 by goroutine 286:
  runtime.mapaccess1_fast32()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/internal/runtime/maps/runtime_fast32_swiss.go:17 +0x24c
  github.com/DataDog/datadog-agent/pkg/process/metadata/parser.(*ServiceExtractor).GetServiceContext()
      /git/datadog-agent/pkg/process/metadata/parser/service.go:175 +0x450
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).addTags()
      /git/datadog-agent/pkg/network/sender/tags.go:134 +0x3d4
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2-range1()
      /git/datadog-agent/pkg/network/sender/sender_linux.go:486 +0xe74
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2.Chunk[go.shape.[]github.com/DataDog/datadog-agent/pkg/network.ConnectionStats,go.shape.e316e00b3fc4fc9ce6e7aff6ec3f690f60e7a5ad809576cc40707cee3ecb6d31].2()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:109 +0x29c
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux.go:427 +0x88
  slices.AppendSeq[go.shape.[]go.shape.*uint8,go.shape.*uint8]()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:51 +0x214
  slices.Collect[go.shape.*uint8]()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:60 +0x9c
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xb8
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4

Previous write at 0x00c001647b60 by goroutine 285:
  runtime.mapaccess2_fast32()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/internal/runtime/maps/runtime_fast32_swiss.go:86 +0x26c
  github.com/DataDog/datadog-agent/pkg/process/metadata/parser.(*ServiceExtractor).ExtractSingle()
      /git/datadog-agent/pkg/process/metadata/parser/service.go:141 +0x230
  github.com/DataDog/datadog-agent/pkg/network/sender.(*serviceExtractor).process()
      /git/datadog-agent/pkg/network/sender/service_extract.go:36 +0x1bc
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSenderConsumer).process()
      /git/datadog-agent/pkg/network/sender/event_consumer_linux.go:158 +0x230
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSenderConsumer).HandleEvent()
      /git/datadog-agent/pkg/network/sender/event_consumer_linux.go:140 +0x44c
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func1()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:527 +0xc0

Goroutine 286 (running) created at:
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:533 +0x400
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:496 +0x30
  testing.tRunner()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1934 +0x164
  testing.(*T).Run.gowrap1()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1997 +0x3c

Goroutine 285 (running) created at:
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:514 +0x324
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:496 +0x30
  testing.tRunner()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1934 +0x164
  testing.(*T).Run.gowrap1()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1997 +0x3c
==================
==================
WARNING: DATA RACE
Read at 0x00c0016532f0 by goroutine 286:
  github.com/DataDog/datadog-agent/pkg/process/metadata/parser.(*ServiceExtractor).GetServiceContext()
      /git/datadog-agent/pkg/process/metadata/parser/service.go:175 +0x45c
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).addTags()
      /git/datadog-agent/pkg/network/sender/tags.go:134 +0x3d4
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2-range1()
      /git/datadog-agent/pkg/network/sender/sender_linux.go:486 +0xe74
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2.Chunk[go.shape.[]github.com/DataDog/datadog-agent/pkg/network.ConnectionStats,go.shape.e316e00b3fc4fc9ce6e7aff6ec3f690f60e7a5ad809576cc40707cee3ecb6d31].2()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:109 +0x29c
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux.go:427 +0x88
  slices.AppendSeq[go.shape.[]go.shape.*uint8,go.shape.*uint8]()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:51 +0x214
  slices.Collect[go.shape.*uint8]()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:60 +0x9c
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xb8
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4

Previous write at 0x00c0016532f0 by goroutine 285:
  github.com/DataDog/datadog-agent/pkg/process/metadata/parser.(*ServiceExtractor).ExtractSingle()
      /git/datadog-agent/pkg/process/metadata/parser/service.go:141 +0x23c
  github.com/DataDog/datadog-agent/pkg/network/sender.(*serviceExtractor).process()
      /git/datadog-agent/pkg/network/sender/service_extract.go:36 +0x1bc
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSenderConsumer).process()
      /git/datadog-agent/pkg/network/sender/event_consumer_linux.go:158 +0x230
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSenderConsumer).HandleEvent()
      /git/datadog-agent/pkg/network/sender/event_consumer_linux.go:140 +0x44c
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func1()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:527 +0xc0

Goroutine 286 (running) created at:
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:533 +0x400
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:496 +0x30
  testing.tRunner()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1934 +0x164
  testing.(*T).Run.gowrap1()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1997 +0x3c

Goroutine 285 (running) created at:
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:514 +0x324
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:496 +0x30
  testing.tRunner()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1934 +0x164
  testing.(*T).Run.gowrap1()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1997 +0x3c
==================
==================
WARNING: DATA RACE
Read at 0x00c000c24b28 by goroutine 286:
  github.com/DataDog/datadog-agent/pkg/process/metadata/parser.(*ServiceExtractor).GetServiceContext()
      /git/datadog-agent/pkg/process/metadata/parser/service.go:176 +0x490
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).addTags()
      /git/datadog-agent/pkg/network/sender/tags.go:134 +0x3d4
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2-range1()
      /git/datadog-agent/pkg/network/sender/sender_linux.go:486 +0xe74
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2.Chunk[go.shape.[]github.com/DataDog/datadog-agent/pkg/network.ConnectionStats,go.shape.e316e00b3fc4fc9ce6e7aff6ec3f690f60e7a5ad809576cc40707cee3ecb6d31].2()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:109 +0x29c
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux.go:427 +0x88
  slices.AppendSeq[go.shape.[]go.shape.*uint8,go.shape.*uint8]()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:51 +0x214
  slices.Collect[go.shape.*uint8]()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:60 +0x9c
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xb8
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4

Previous write at 0x00c000c24b28 by goroutine 285:
  github.com/DataDog/datadog-agent/pkg/process/metadata/parser.(*ServiceExtractor).extractServiceMetadata()
      /git/datadog-agent/pkg/process/metadata/parser/service.go:235 +0x2dc
  github.com/DataDog/datadog-agent/pkg/process/metadata/parser.(*ServiceExtractor).ExtractSingle()
      /git/datadog-agent/pkg/process/metadata/parser/service.go:136 +0x184
  github.com/DataDog/datadog-agent/pkg/network/sender.(*serviceExtractor).process()
      /git/datadog-agent/pkg/network/sender/service_extract.go:36 +0x1bc
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSenderConsumer).process()
      /git/datadog-agent/pkg/network/sender/event_consumer_linux.go:158 +0x230
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSenderConsumer).HandleEvent()
      /git/datadog-agent/pkg/network/sender/event_consumer_linux.go:140 +0x44c
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func1()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:527 +0xc0

Goroutine 286 (running) created at:
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:533 +0x400
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:496 +0x30
  testing.tRunner()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1934 +0x164
  testing.(*T).Run.gowrap1()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1997 +0x3c

Goroutine 285 (running) created at:
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:514 +0x324
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:496 +0x30
  testing.tRunner()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1934 +0x164
  testing.(*T).Run.gowrap1()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1997 +0x3c
==================
==================
WARNING: DATA RACE
Read at 0x00c001647b60 by goroutine 286:
  runtime.mapaccess1_fast32()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/internal/runtime/maps/runtime_fast32_swiss.go:17 +0x24c
  github.com/DataDog/datadog-agent/pkg/process/metadata/parser.(*ServiceExtractor).GetServiceContext()
      /git/datadog-agent/pkg/process/metadata/parser/service.go:175 +0x450
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).addTags()
      /git/datadog-agent/pkg/network/sender/tags.go:134 +0x3d4
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2-range1()
      /git/datadog-agent/pkg/network/sender/sender_linux.go:486 +0xe74
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2.Chunk[go.shape.[]github.com/DataDog/datadog-agent/pkg/network.ConnectionStats,go.shape.e316e00b3fc4fc9ce6e7aff6ec3f690f60e7a5ad809576cc40707cee3ecb6d31].2()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:109 +0x29c
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux.go:427 +0x88
  slices.AppendSeq[go.shape.[]go.shape.*uint8,go.shape.*uint8]()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:51 +0x214
  slices.Collect[go.shape.*uint8]()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:60 +0x9c
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xb8
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4

Previous write at 0x00c001647b60 by goroutine 285:
  runtime.mapaccess2_fast32()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/internal/runtime/maps/runtime_fast32_swiss.go:86 +0x26c
  github.com/DataDog/datadog-agent/pkg/process/metadata/parser.(*ServiceExtractor).ExtractSingle()
      /git/datadog-agent/pkg/process/metadata/parser/service.go:141 +0x230
  github.com/DataDog/datadog-agent/pkg/network/sender.(*serviceExtractor).process()
      /git/datadog-agent/pkg/network/sender/service_extract.go:36 +0x1bc
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSenderConsumer).process()
      /git/datadog-agent/pkg/network/sender/event_consumer_linux.go:158 +0x230
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSenderConsumer).HandleEvent()
      /git/datadog-agent/pkg/network/sender/event_consumer_linux.go:140 +0x44c
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func1()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:527 +0xc0

Goroutine 286 (running) created at:
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:533 +0x400
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:496 +0x30
  testing.tRunner()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1934 +0x164
  testing.(*T).Run.gowrap1()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1997 +0x3c

Goroutine 285 (running) created at:
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:514 +0x324
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:496 +0x30
  testing.tRunner()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1934 +0x164
  testing.(*T).Run.gowrap1()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1997 +0x3c
==================
==================
WARNING: DATA RACE
Read at 0x00c000f36968 by goroutine 286:
  github.com/DataDog/datadog-agent/pkg/process/metadata/parser.(*ServiceExtractor).GetServiceContext()
      /git/datadog-agent/pkg/process/metadata/parser/service.go:175 +0x45c
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).addTags()
      /git/datadog-agent/pkg/network/sender/tags.go:134 +0x3d4
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2-range1()
      /git/datadog-agent/pkg/network/sender/sender_linux.go:486 +0xe74
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2.Chunk[go.shape.[]github.com/DataDog/datadog-agent/pkg/network.ConnectionStats,go.shape.e316e00b3fc4fc9ce6e7aff6ec3f690f60e7a5ad809576cc40707cee3ecb6d31].2()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:109 +0x29c
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux.go:427 +0x88
  slices.AppendSeq[go.shape.[]go.shape.*uint8,go.shape.*uint8]()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:51 +0x214
  slices.Collect[go.shape.*uint8]()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:60 +0x9c
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xb8
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4

Previous write at 0x00c000f36968 by goroutine 285:
  github.com/DataDog/datadog-agent/pkg/process/metadata/parser.(*ServiceExtractor).ExtractSingle()
      /git/datadog-agent/pkg/process/metadata/parser/service.go:141 +0x23c
  github.com/DataDog/datadog-agent/pkg/network/sender.(*serviceExtractor).process()
      /git/datadog-agent/pkg/network/sender/service_extract.go:36 +0x1bc
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSenderConsumer).process()
      /git/datadog-agent/pkg/network/sender/event_consumer_linux.go:158 +0x230
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSenderConsumer).HandleEvent()
      /git/datadog-agent/pkg/network/sender/event_consumer_linux.go:140 +0x44c
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func1()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:527 +0xc0

Goroutine 286 (running) created at:
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:533 +0x400
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:496 +0x30
  testing.tRunner()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1934 +0x164
  testing.(*T).Run.gowrap1()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1997 +0x3c

Goroutine 285 (running) created at:
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:514 +0x324
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:496 +0x30
  testing.tRunner()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1934 +0x164
  testing.(*T).Run.gowrap1()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1997 +0x3c
==================
==================
WARNING: DATA RACE
Read at 0x00c001647b60 by goroutine 286:
  runtime.mapaccess1_fast32()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/internal/runtime/maps/runtime_fast32_swiss.go:17 +0x24c
  github.com/DataDog/datadog-agent/pkg/process/metadata/parser.(*ServiceExtractor).GetServiceContext()
      /git/datadog-agent/pkg/process/metadata/parser/service.go:175 +0x450
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).addTags()
      /git/datadog-agent/pkg/network/sender/tags.go:134 +0x3d4
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2-range1()
      /git/datadog-agent/pkg/network/sender/sender_linux.go:486 +0xe74
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2.Chunk[go.shape.[]github.com/DataDog/datadog-agent/pkg/network.ConnectionStats,go.shape.e316e00b3fc4fc9ce6e7aff6ec3f690f60e7a5ad809576cc40707cee3ecb6d31].2()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:109 +0x29c
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux.go:427 +0x88
  slices.AppendSeq[go.shape.[]go.shape.*uint8,go.shape.*uint8]()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:51 +0x214
  slices.Collect[go.shape.*uint8]()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:60 +0x9c
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xb8
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4

Previous write at 0x00c001647b60 by goroutine 285:
  runtime.mapaccess2_fast32()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/internal/runtime/maps/runtime_fast32_swiss.go:86 +0x26c
  github.com/DataDog/datadog-agent/pkg/process/metadata/parser.(*ServiceExtractor).ExtractSingle()
      /git/datadog-agent/pkg/process/metadata/parser/service.go:141 +0x230
  github.com/DataDog/datadog-agent/pkg/network/sender.(*serviceExtractor).process()
      /git/datadog-agent/pkg/network/sender/service_extract.go:36 +0x1bc
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSenderConsumer).process()
      /git/datadog-agent/pkg/network/sender/event_consumer_linux.go:158 +0x230
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSenderConsumer).HandleEvent()
      /git/datadog-agent/pkg/network/sender/event_consumer_linux.go:140 +0x44c
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func1()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:527 +0xc0

Goroutine 286 (running) created at:
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:533 +0x400
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:496 +0x30
  testing.tRunner()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1934 +0x164
  testing.(*T).Run.gowrap1()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1997 +0x3c

Goroutine 285 (running) created at:
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:514 +0x324
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:496 +0x30
  testing.tRunner()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1934 +0x164
  testing.(*T).Run.gowrap1()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1997 +0x3c
==================
==================
WARNING: DATA RACE
Read at 0x00c000f37180 by goroutine 286:
  github.com/DataDog/datadog-agent/pkg/process/metadata/parser.(*ServiceExtractor).GetServiceContext()
      /git/datadog-agent/pkg/process/metadata/parser/service.go:175 +0x45c
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).addTags()
      /git/datadog-agent/pkg/network/sender/tags.go:134 +0x3d4
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2-range1()
      /git/datadog-agent/pkg/network/sender/sender_linux.go:486 +0xe74
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2.Chunk[go.shape.[]github.com/DataDog/datadog-agent/pkg/network.ConnectionStats,go.shape.e316e00b3fc4fc9ce6e7aff6ec3f690f60e7a5ad809576cc40707cee3ecb6d31].2()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:109 +0x29c
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSender).batches.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux.go:427 +0x88
  slices.AppendSeq[go.shape.[]go.shape.*uint8,go.shape.*uint8]()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:51 +0x214
  slices.Collect[go.shape.*uint8]()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/slices/iter.go:60 +0x9c
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xb8
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func2()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:536 +0xc4

Previous write at 0x00c000f37180 by goroutine 285:
  github.com/DataDog/datadog-agent/pkg/process/metadata/parser.(*ServiceExtractor).ExtractSingle()
      /git/datadog-agent/pkg/process/metadata/parser/service.go:141 +0x23c
  github.com/DataDog/datadog-agent/pkg/network/sender.(*serviceExtractor).process()
      /git/datadog-agent/pkg/network/sender/service_extract.go:36 +0x1bc
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSenderConsumer).process()
      /git/datadog-agent/pkg/network/sender/event_consumer_linux.go:158 +0x230
  github.com/DataDog/datadog-agent/pkg/network/sender.(*directSenderConsumer).HandleEvent()
      /git/datadog-agent/pkg/network/sender/event_consumer_linux.go:140 +0x44c
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess.func1()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:527 +0xc0

Goroutine 286 (running) created at:
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:533 +0x400
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:496 +0x30
  testing.tRunner()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1934 +0x164
  testing.(*T).Run.gowrap1()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1997 +0x3c

Goroutine 285 (running) created at:
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:514 +0x324
  github.com/DataDog/datadog-agent/pkg/network/sender.TestServiceExtractorConcurrentAccess()
      /git/datadog-agent/pkg/network/sender/sender_linux_test.go:496 +0x30
  testing.tRunner()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1934 +0x164
  testing.(*T).Run.gowrap1()
      /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.6.linux-arm64/src/testing/testing.go:1997 +0x3c
==================
    testing.go:1617: race detected during execution of test
--- FAIL: TestServiceExtractorConcurrentAccess (9.15s)
FAIL
FAIL	github.com/DataDog/datadog-agent/pkg/network/sender	9.257s
FAIL
