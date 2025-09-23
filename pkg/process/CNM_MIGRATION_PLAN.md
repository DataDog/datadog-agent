# Migration Summary: Moving NPM/USM Connection Logic from Process-Agent to System-Probe

**SCOPE: Linux Platform Only** - This migration is scoped exclusively for Linux environments where `process_config.run_in_core_agent.enabled = true` by default.

## **Current Architecture**
```
Core Agent (when process_config.run_in_core_agent.enabled = true, default on Linux):
├── Process Collection (WorkloadMeta collector) → scans /proc, populates wmeta
├── Service Discovery → uses process data from wmeta
└── Language Detection → uses process data from wmeta

Process-Agent:
├── ConnectionsCheck.getConnections() → HTTP call to system-probe /connections
├── ProcessData.Fetch() → DUPLICATE /proc scan ⚠️
├── DockerFilter + ServiceExtractor → uses duplicate process data
├── Container & host metadata enrichment 
├── Payload assembly & batching (CollectorConnections protobuf)
└── npCollector.ScheduleConns() → schedules traceroutes for Network Path Monitoring

System-Probe:
├── tracer.GetActiveConnections() → returns raw connection data
└── Traceroute module → executes traceroutes via /traceroute/{host} endpoint
```

## **Target Architecture**
```
Core Agent:
└── Process Collection (WorkloadMeta collector) → scans /proc, populates wmeta

System-Probe:
├── tracer.GetActiveConnections() → raw connections (already exists)
├── WorkloadMeta Remote Client → reads process entities from wmeta ✅
├── Process context enrichment → uses wmeta process data (MIGRATE)
├── Container & host metadata enrichment (MIGRATE from process-agent)  
├── Payload assembly & batching (MIGRATE from process-agent)
├── npCollector component (MIGRATE from process-agent)
└── Traceroute module → executes traceroutes (already exists)

Process-Agent:
└── [Eliminated - no longer needed for NPM/USM]
```

## **Migration Benefits**
- **Performance**: Eliminates duplicate `/proc` scanning between core-agent and process-agent
- **Architecture**: Single source of truth for process data via WorkloadMeta
- **Resource Usage**: Reduces memory and CPU overhead from redundant process collection
- **Cross-platform**: `process_config.run_in_core_agent.enabled` defaults to `true` on Linux, `false` on Windows/macOS

## **Components to Migrate**

### **1. Process Context Enrichment & Filtering** ✅ **MIGRATE - LOW COMPLEXITY**
**Migration Strategy**: Replace `ProcessData.Fetch()` with WorkloadMeta process entity queries
- **DockerFilter.Filter()**: Remove docker-proxy connections using wmeta process data
- **ServiceExtractor.GetServiceContext()**: Infer service names using wmeta process cmdlines
  - ⚠️ **Evaluation Required**: Assess actual usage of `process_context` tags in production
  - **Potential Simplification**: May have very low adoption and could be dropped entirely
- **LocalResolver.Resolve()**: Resolve container endpoints using wmeta container data
- **Dependency Note**: Requires `process_config.run_in_core_agent.enabled = true` (default on Linux)

### **2. Container & Host Metadata Enrichment** ✅ **MIGRATE - MEDIUM COMPLEXITY**
**Files**: `pkg/process/checks/net.go` (lines 188, 199-233)
- **Container tagging**: `getContainerTagsCallback()` with workloadmeta integration
- **Host tags**: `hostTagProvider.GetHostTags()` 
- **Host info**: OS/kernel version, platform, architecture
- **Explicit tagging**: Container startup time-based tagging logic
- **Migration Strategy**: Use existing system-probe wmeta + tagger components

### **3. Payload Assembly & Batching** ✅ **MIGRATE - LOW COMPLEXITY**
**Files**: `pkg/process/checks/net.go` (lines 373-551)
- **DNS encoding**: Complex V2 DNS statistics encoding and remapping
- **Connection batching**: Split connections into intake-sized chunks (`maxConnsPerMessage`)
- **Tag encoding**: V3 tag encoding with deduplication
- **Route optimization**: Network route index remapping  
- **CollectorConnections creation**: Final protobuf message assembly
- **Telemetry assembly**: Runtime compilation, eBPF assets, kernel compatibility
- **Migration Strategy**: Pure algorithmic logic migration (self-contained)

### **4. Network Path Collector (npCollector)** ⚠️ **MIGRATE - MEDIUM COMPLEXITY**
**Files**: `comp/networkpath/npcollector/npcollectorimpl/`
- **Connection filtering**: VPC, CIDR, protocol, direction filtering
- **Traceroute scheduling**: Queue management and worker pools
- **System-probe integration**: HTTP calls to `/traceroute/{host}` 
- **Event publishing**: Send network path data to Event Platform
- **Migration Strategy**: Migrate entire component, requires EventPlatform forwarder dependency

## **Dependencies to Migrate**

### **External Components Required**
- **Workloadmeta**: Container metadata (`comp/core/workloadmeta`)
- **Tagger**: Container and host tagging (`comp/core/tagger`)  
- **HostTagProvider**: Host-level tags (`pkg/hosttags`)
- **Event Platform Forwarder**: For npCollector output (`comp/forwarder/eventplatform`)
- **Reverse DNS Querier**: For npCollector (`comp/rdnsquerier`)

### **Configuration Migration**
- Process service inference configs (`system_probe_config.process_service_inference.*`)
- Network path monitoring configs (`network_path.*`)
- Container provider and resolver configs
- Expected tags duration and explicit tagging settings

## **Key Implementation Notes**

### **WorkloadMeta Dependency**
- **Critical**: Migration depends on `process_config.run_in_core_agent.enabled = true` (default on Linux)
- **Cross-platform**: Defaults to `false` on Windows/macOS, may need configuration consideration
- **Alternative**: On Windows/macOS, may need to migrate existing process scanning logic to system-probe

### **Component Interdependencies** 
- **System-probe already has**: WorkloadMeta client, container monitoring, tagger access
- **Need to add**: EventPlatform forwarder, Reverse DNS querier (for npCollector)
- **Architecture benefit**: Eliminates duplicate process scanning, unified data flow