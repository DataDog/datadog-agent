// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config holds config related files
//
//go:generate go run github.com/DataDog/datadog-agent/pkg/security/generators/config_doc -input config.go -output ../../../docs/cloud-workload-security/workload_protection_agent_config.schema.json
package config

import (
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"slices"
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	pconfig "github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
)

const (
	// ADMinMaxDumSize represents the minimum value for runtime_security_config.activity_dump.max_dump_size
	ADMinMaxDumSize = 100
)

var (
	// defaultKernelCompilationFlags are the kernel compilation flags checked by CWS
	// This list was moved here, away from the config/ package, to reduce the size of the agent metadata payload in REDAPL.
	defaultKernelCompilationFlags = []string{
		// memory management hardening
		"CONFIG_DEBUG_KERNEL",
		"CONFIG_DEBUG_RODATA",
		"CONFIG_STRICT_KERNEL_RWX",
		"CONFIG_ARCH_OPTIONAL_KERNEL_RWX",
		"CONFIG_ARCH_HAS_STRICT_KERNEL_RWX",
		"CONFIG_DEBUG_WX",
		"CONFIG_INIT_ON_ALLOC_DEFAULT_ON",
		"CONFIG_INIT_ON_FREE_DEFAULT_ON",
		"CONFIG_IOMMU_DEFAULT_DMA_STRICT",
		"CONFIG_IOMMU_DEFAULT_PASSTHROUGH",
		"CONFIG_KFENCE",
		"CONFIG_HAVE_ARCH_KFENCE",
		"CONFIG_KFENCE_SAMPLE_INTERVAL",
		"CONFIG_RANDOMIZE_KSTACK_OFFSET_DEFAULT",
		"CONFIG_CC_STACKPROTECTOR",
		"CONFIG_CC_STACKPROTECTOR_STRONG",
		"CONFIG_STACKPROTECTOR",
		"CONFIG_STACKPROTECTOR_STRONG",
		"CONFIG_DEVMEM",
		"CONFIG_STRICT_DEVMEM",
		"CONFIG_IO_STRICT_DEVMEM",
		"CONFIG_SCHED_STACK_END_CHECK",
		"CONFIG_HARDENED_USERCOPY",
		"CONFIG_HARDENED_USERCOPY_FALLBACK",
		"CONFIG_HARDENED_USERCOPY_PAGESPAN",
		"CONFIG_VMAP_STACK",
		"CONFIG_REFCOUNT_FULL",
		"CONFIG_FORTIFY_SOURCE",
		"CONFIG_ACPI_CUSTOM_METHOD",
		"CONFIG_DEVKMEM",
		"CONFIG_PROC_KCORE",
		"CONFIG_COMPAT_VDSO",
		"CONFIG_SECURITY_DMESG_RESTRICT",
		"CONFIG_RETPOLINE",
		"CONFIG_LEGACY_VSYSCALL_NONE",
		"CONFIG_LEGACY_VSYSCALL_EMULATE",
		"CONFIG_LEGACY_VSYSCALL_XONLY",
		"CONFIG_X86_VSYSCALL_EMULATION",
		"CONFIG_SHUFFLE_PAGE_ALLOCATOR",
		// protect kernel data structures
		"CONFIG_DEBUG_CREDENTIALS",
		"CONFIG_DEBUG_NOTIFIERS",
		"CONFIG_DEBUG_LIST",
		"CONFIG_DEBUG_SG",
		"CONFIG_BUG_ON_DATA_CORRUPTION",
		// harden the memory allocator
		"CONFIG_SLAB_FREELIST_RANDOM",
		"CONFIG_SLUB",
		"CONFIG_SLAB_FREELIST_HARDENED",
		"CONFIG_SLAB_MERGE_DEFAULT",
		"CONFIG_SLUB_DEBUG",
		"CONFIG_PAGE_POISONING",
		"CONFIG_PAGE_POISONING_NO_SANITY",
		"CONFIG_PAGE_POISONING_ZERO",
		"CONFIG_COMPAT_BRK",
		// harden the management of kernel modules
		"CONFIG_MODULES",
		"CONFIG_STRICT_MODULE_RWX",
		"CONFIG_MODULE_SIG",
		"CONFIG_MODULE_SIG_FORCE",
		"CONFIG_MODULE_SIG_ALL",
		"CONFIG_MODULE_SIG_SHA512",
		"CONFIG_MODULE_SIG_HASH",
		// handle abnormal situations
		"CONFIG_BUG",
		"CONFIG_PANIC_ON_OOPS",
		"CONFIG_PANIC_TIMEOUT",
		// configure kernel security functions
		"CONFIG_MULTIUSER",
		"CONFIG_SECURITY",
		"CONFIG_SECCOMP",
		"CONFIG_SECCOMP_FILTER",
		"CONFIG_SECURITY_YAMA",
		"CONFIG_SECURITY_WRITABLE_HOOKS",
		"CONFIG_SECURITY_SELINUX",
		"CONFIG_SECURITY_SELINUX_DISABLE",
		"CONFIG_BPF_LSM",
		"CONFIG_SECURITY_SMACK",
		"CONFIG_SECURITY_TOMOYO",
		"CONFIG_SECURITY_APPARMOR",
		"CONFIG_SECURITY_LOCKDOWN_LSM",
		"CONFIG_LOCK_DOWN_KERNEL_FORCE_INTEGRITY",
		"CONFIG_SECURITY_LANDLOCK",
		"CONFIG_LSM",
		"CONFIG_INTEGRITY_AUDIT",
		"CONFIG_INTEGRITY",
		"CONFIG_INTEGRITY_SIGNATURE",
		"CONFIG_INTEGRITY_ASYMMETRIC_KEYS",
		"CONFIG_INTEGRITY_TRUSTED_KEYRING",
		"CONFIG_INTEGRITY_PLATFORM_KEYRING",
		"CONFIG_INTEGRITY_MACHINE_KEYRING",
		// configure compiler plugins
		"CONFIG_GCC_PLUGINS",
		"CONFIG_GCC_PLUGIN_LATENT_ENTROPY",
		"CONFIG_GCC_PLUGIN_STACKLEAK",
		"CONFIG_GCC_PLUGIN_STRUCTLEAK",
		"CONFIG_GCC_PLUGIN_STRUCTLEAK_BYREF_ALL",
		"CONFIG_GCC_PLUGIN_RANDSTRUCT",
		"CONFIG_GCC_PLUGIN_RANDSTRUCT_PERFORMANCE",
		// configure the IP stack
		"CONFIG_IPV6",
		"CONFIG_SYN_COOKIES",
		// various kernel behavior
		"CONFIG_KEXEC",
		"CONFIG_KEXEC_SIG",
		"CONFIG_KEXEC_SIG_FORCE",
		"CONFIG_HIBERNATION",
		"CONFIG_BINFMT_MISC",
		"CONFIG_LEGACY_PTYS",
		// options for x86_32 bit architectures
		"CONFIG_HIGHMEM64G",
		"CONFIG_X86_PAE",
		// options for x86_64 or arm64 architectures
		"CONFIG_X86_64",
		"CONFIG_DEFAULT_MMAP_MIN_ADDR",
		"CONFIG_RANDOMIZE_BASE",
		"CONFIG_RANDOMIZE_MEMORY",
		"CONFIG_PAGE_TABLE_ISOLATION",
		"CONFIG_IA32_EMULATION",
		"CONFIG_MODIFY_LDT_SYSCALL",
		"CONFIG_ARM64_SW_TTBR0_PAN",
		"CONFIG_UNMAP_KERNEL_AT_EL0",
	}
)

// Policy represents a policy file in the configuration file
type Policy struct {
	Name  string   `mapstructure:"name"`
	Files []string `mapstructure:"files"`
	Tags  []string `mapstructure:"tags"`
}

// RuntimeSecurityConfig holds the configuration for the runtime security agent
type RuntimeSecurityConfig struct {
	// description: Defines if the runtime security module should be enabled
	// visibility: public
	// default_value: false
	RuntimeEnabled bool

	// description: PoliciesDir defines the folder in which the policy files are located
	// visibility: private
	// default_value: /etc/datadog-agent/runtime-security.d
	PoliciesDir string

	// description: PolicyMonitorEnabled enable policy monitoring
	// visibility: private
	// default_value: false
	PolicyMonitorEnabled bool

	// description: PolicyMonitorPerRuleEnabled enabled per-rule policy monitoring
	// visibility: private
	// default_value: false
	PolicyMonitorPerRuleEnabled bool

	// description: PolicyMonitorReportInternalPolicies enable internal policies monitoring
	// visibility: private
	// default_value: false
	PolicyMonitorReportInternalPolicies bool

	// description: RuleCacheEnabled defines if the rule cache should be enabled
	// visibility: private
	// default_value: true
	RuleCacheEnabled bool

	// description: SocketPath is the path to the socket that is used to communicate with the security agent
	// visibility: private
	// default_value: ${install_path}/run/runtime-security.sock
	SocketPath string

	// description: SocketPath is the path to the socket that is used to communicate with system-probe
	// visibility: private
	// default_value: ""
	CmdSocketPath string

	// description: EventServerBurst defines the maximum burst of events that can be sent over the grpc server
	// visibility: private
	// default_value: 40
	EventServerBurst int

	// description: EventServerRate defines the grpc server rate at which events can be sent
	// visibility: private
	// default_value: 10
	EventServerRate int

	// description: EventServerRetention defines an event retention period so that some fields can be resolved
	// visibility: private
	// default_value: 6s
	EventServerRetention time.Duration

	// description: EventRetryQueueThreshold defines the maximum size of the event queue after which we force sending events even if not resolved
	// visibility: private
	// default_value: 512
	EventRetryQueueThreshold int

	// description: FIMEnabled determines whether fim rules will be loaded
	// visibility: private
	// default_value: false
	FIMEnabled bool

	// description: SelfTestEnabled defines if the self tests should be executed at startup or not
	// visibility: private
	// default_value: true
	SelfTestEnabled bool

	// description: SelfTestSendReport defines if a self test event will be emitted
	// visibility: private
	// default_value: true
	SelfTestSendReport bool

	// description: RemoteConfigurationEnabled defines whether to use remote monitoring
	// visibility: private
	// default_value: true
	RemoteConfigurationEnabled bool

	// description: RemoteConfigurationDumpPolicies defines whether to dump remote config policy
	// visibility: public
	// default_value: false
	RemoteConfigurationDumpPolicies bool

	// description: LogPatterns pattern to be used by the logger for trace level
	// visibility: private
	// default_value: []
	LogPatterns []string

	// description: LogTags tags to be used by the logger for trace level
	// visibility: private
	// default_value: []
	LogTags []string

	// description: EnvAsTags convert envs to tags
	// visibility: private
	// default_value: []
	EnvAsTags []string

	// description: HostServiceName string
	// visibility: private
	// default_value: ""
	HostServiceName string

	// description: OnDemandEnabled defines whether the on-demand probes should be enabled
	// visibility: private
	// default_value: true
	OnDemandEnabled bool

	// description: OnDemandRateLimiterEnabled defines whether the on-demand probes rate limit getting hit disabled the on demand probes
	// visibility: private
	// default_value: true
	OnDemandRateLimiterEnabled bool

	// description: ReducedProcPidCacheSize defines whether the `proc_cache` and `pid_cache` map should use reduced size
	// visibility: private
	// default_value: false
	ReducedProcPidCacheSize bool

	// description: InternalMonitoringEnabled determines if the monitoring events of the agent should be sent to Datadog
	// visibility: private
	// default_value: false
	InternalMonitoringEnabled bool

	// description: ActivityDumpEnabled defines if the activity dump manager should be enabled
	// visibility: private
	// default_value: true
	ActivityDumpEnabled bool

	// description: ActivityDumpCleanupPeriod defines the period at which the activity dump manager should perform its cleanup operation.
	// visibility: private
	// default_value: 30s
	ActivityDumpCleanupPeriod time.Duration

	// description: ActivityDumpTagsResolutionPeriod defines the period at which the activity dump manager should try to resolve missing container tags.
	// visibility: private
	// default_value: 60s
	ActivityDumpTagsResolutionPeriod time.Duration

	// description: ActivityDumpLoadControlPeriod defines the period at which the activity dump manager should trigger the load controller
	// visibility: private
	// default_value: 60s
	ActivityDumpLoadControlPeriod time.Duration

	// description: ActivityDumpLoadControlMinDumpTimeout defines minimal duration of a activity dump recording
	// visibility: private
	// default_value: 10m
	ActivityDumpLoadControlMinDumpTimeout time.Duration

	// description: ActivityDumpTracedCgroupsCount defines the maximum count of cgroups that should be monitored concurrently. Leave this parameter to 0 to prevent the generation of activity dumps based on cgroups.
	// visibility: private
	// default_value: 5
	ActivityDumpTracedCgroupsCount int

	// description: ActivityDumpTraceSystemdCgroups defines if you want to trace systemd cgroups
	// visibility: private
	// default_value: false
	ActivityDumpTraceSystemdCgroups bool

	// description: ActivityDumpTracedEventTypes defines the list of events that should be captured in an activity dump. Leave this parameter empty to monitor all event types. If not already present, the `exec` event will automatically be added to this list.
	// visibility: private
	// default_value: ["exec", "open", "dns", "imds"]
	ActivityDumpTracedEventTypes []model.EventType

	// description: ActivityDumpCgroupDumpTimeout defines the cgroup activity dumps timeout.
	// visibility: private
	// default_value: 900s
	ActivityDumpCgroupDumpTimeout time.Duration

	// description: ActivityDumpRateLimiter defines the kernel rate of max events per sec for activity dumps.
	// visibility: private
	// default_value: 500
	ActivityDumpRateLimiter uint16

	// description: ActivityDumpCgroupWaitListTimeout defines the time to wait before a cgroup can be dumped again.
	// visibility: private
	// default_value: 4500s
	ActivityDumpCgroupWaitListTimeout time.Duration

	// description: ActivityDumpCgroupDifferentiateArgs defines if system-probe should differentiate process nodes using process arguments for dumps.
	// visibility: private
	// default_value: false
	ActivityDumpCgroupDifferentiateArgs bool

	// description: ActivityDumpLocalStorageDirectory defines the output directory for the activity dumps and graphs. Leave this field empty to prevent writing any output to disk.
	// visibility: private
	// default_value: ${run_path}/runtime-security/profiles
	ActivityDumpLocalStorageDirectory string

	// description: ActivityDumpLocalStorageFormats defines the formats that should be used to persist the activity dumps locally.
	// visibility: private
	// default_value: ["profile"]
	ActivityDumpLocalStorageFormats []StorageFormat

	// description: ActivityDumpLocalStorageCompression defines if the local storage should compress the persisted data.
	// visibility: private
	// default_value: false
	ActivityDumpLocalStorageCompression bool

	// description: ActivityDumpLocalStorageMaxDumpsCount defines the maximum count of activity dumps that should be kept locally. When the limit is reached, the oldest dumps will be deleted first.
	// visibility: private
	// default_value: 100
	ActivityDumpLocalStorageMaxDumpsCount int

	// description: ActivityDumpSyscallMonitorPeriod defines the minimum amount of time to wait between 2 syscalls event for the same process.
	// visibility: private
	// default_value: 60s
	ActivityDumpSyscallMonitorPeriod time.Duration

	// description: ActivityDumpMaxDumpCountPerWorkload defines the maximum amount of dumps that the agent should send for a workload
	// visibility: private
	// default_value: 25
	ActivityDumpMaxDumpCountPerWorkload int

	// description: ActivityDumpWorkloadDenyList defines the list of workloads for which we shouldn't generate dumps. Workloads should be provided as strings in the following format "{image_name}:[{image_tag}|*]". If "*" is provided instead of a specific image tag, then the entry will match any workload with the input {image_name} regardless of their tag.
	// visibility: private
	// default_value: []
	ActivityDumpWorkloadDenyList []string

	// description: ActivityDumpTagRulesEnabled enable the tagging of nodes with matched rules
	// visibility: private
	// default_value: true
	ActivityDumpTagRulesEnabled bool

	// description: ActivityDumpSilentWorkloadsDelay defines the minimum amount of time to wait before the activity dump manager will start tracing silent workloads
	// visibility: private
	// default_value: 10s
	ActivityDumpSilentWorkloadsDelay time.Duration

	// description: ActivityDumpSilentWorkloadsTicker configures ticker that will check if a workload is silent and should be traced
	// visibility: private
	// default_value: 10s
	ActivityDumpSilentWorkloadsTicker time.Duration

	// # Dynamic configuration fields:

	// description: ActivityDumpMaxDumpSize defines the maximum size of a dump
	// visibility: private
	// default_value: 1750
	ActivityDumpMaxDumpSize func() int

	// Per-type event sampling config
	// description: EventSamplingOpenEnabled defines if the agent should sample open events
	// visibility: private
	// default_value: false
	EventSamplingOpenEnabled bool

	// description: EventSamplingOpenRate defines the rate at which the agent should sample open events
	// visibility: private
	// default_value: 500
	EventSamplingOpenRate int

	// description: EventSamplingConnectEnabled defines if the agent should sample connect events
	// visibility: private
	// default_value: false
	EventSamplingConnectEnabled bool

	// description: EventSamplingConnectRate defines the rate at which the agent should sample connect events
	// visibility: private
	// default_value: 500
	EventSamplingConnectRate int

	// description: EventSamplingBindEnabled defines if the agent should sample bind events
	// visibility: private
	// default_value: false
	EventSamplingBindEnabled bool

	// description: EventSamplingBindRate defines the rate at which the agent should sample bind events
	// visibility: private
	// default_value: 500
	EventSamplingBindRate int

	// description: EventSamplingDNSEnabled defines if the agent should sample DNS events
	// visibility: private
	// default_value: false
	EventSamplingDNSEnabled bool

	// description: EventSamplingDNSRate defines the rate at which the agent should sample DNS events
	// visibility: private
	// default_value: 500
	EventSamplingDNSRate int

	// description: SecurityProfileEnabled defines if the Security Profile manager should be enabled
	// visibility: private
	// default_value: true
	SecurityProfileEnabled bool

	// description: SecurityProfileManagerV2Enabled defines if the v2 Security Profile manager should be used
	// visibility: private
	// default_value: false
	SecurityProfileV2Enabled bool

	// description: SecurityProfileMaxImageTags defines the maximum number of profile versions to maintain
	// visibility: private
	// default_value: 20
	SecurityProfileMaxImageTags int

	// description: SecurityProfileDir defines the directory in which Security Profiles are stored
	// visibility: private
	// default_value: ${run_path}/runtime-security/profiles
	SecurityProfileDir string

	// description: SecurityProfileWatchDir defines if the Security Profiles directory should be monitored
	// visibility: private
	// default_value: true
	SecurityProfileWatchDir bool

	// description: SecurityProfileCacheSize defines the count of Security Profiles held in cache
	// visibility: private
	// default_value: 10
	SecurityProfileCacheSize int

	// description: SecurityProfileMaxCount defines the maximum number of Security Profiles that may be evaluated concurrently
	// visibility: private
	// default_value: 400
	SecurityProfileMaxCount int

	// description: SecurityProfileDNSMatchMaxDepth defines the max depth of subdomain to be matched for DNS anomaly detection (0 to match everything)
	// visibility: private
	// default_value: 3
	SecurityProfileDNSMatchMaxDepth int

	// description: SecurityProfileNodeEvictionTimeout defines the timeout after which non-touched nodes are evicted from profiles
	// visibility: private
	// default_value: 0s
	SecurityProfileNodeEvictionTimeout time.Duration

	// description: SecurityProfileSampleRefreshPeriod defines the minimum interval between sample refresh events for the same dedup cookie
	// visibility: private
	// default_value: 30s
	SecurityProfileSampleRefreshPeriod time.Duration

	// description: SecurityProfileCleanupDelay defines the delay before removing a profile after all its cgroups are deleted
	// visibility: private
	// default_value: 60m
	SecurityProfileCleanupDelay time.Duration

	// description: SecurityProfileV2EventTypes defines the list of event types that should be captured by the V2 security profile manager
	// visibility: private
	// default_value: ["exec", "dns", "bind", "connect", "open"]
	SecurityProfileV2EventTypes []model.EventType

	// description: SecurityProfileV2ExcludedImages defines the list of "image_name:image_tag" entries excluded from V2 profiling. The tag may be "*" to match any tag for the given image name.
	// visibility: private
	// default_value: []
	SecurityProfileV2ExcludedImages []string

	// description: SecurityProfileV2MaxDumpSize returns the V2-only max profile size in bytes.
	// visibility: private
	// default_value: 5120
	SecurityProfileV2MaxDumpSize func() int

	// description: AnomalyDetectionEventTypes defines the list of events that should be allowed to generate anomaly detections
	// visibility: private
	// default_value: ["exec"]
	AnomalyDetectionEventTypes []model.EventType

	// description: AnomalyDetectionDefaultMinimumStablePeriod defines the default minimum amount of time during which the events that diverge from their profiles are automatically added in their profiles without triggering an anomaly detection event.
	// visibility: private
	// default_value: 900s
	AnomalyDetectionDefaultMinimumStablePeriod time.Duration

	// description: AnomalyDetectionMinimumStablePeriods defines the minimum amount of time per event type during which the events that diverge from their profiles are automatically added in their profiles without triggering an anomaly detection event.
	// visibility: private
	// default_value: {"exec": "900s", "dns": "900s"}
	AnomalyDetectionMinimumStablePeriods map[model.EventType]time.Duration

	// description: AnomalyDetectionUnstableProfileTimeThreshold defines the maximum amount of time to wait until a profile that hasn't reached a stable state is considered as unstable.
	// visibility: private
	// default_value: 1h
	AnomalyDetectionUnstableProfileTimeThreshold time.Duration

	// description: AnomalyDetectionUnstableProfileSizeThreshold defines the maximum size a profile can reach past which it is considered unstable
	// visibility: private
	// default_value: 5000000
	AnomalyDetectionUnstableProfileSizeThreshold int64

	// description: AnomalyDetectionWorkloadWarmupPeriod defines the duration we ignore the anomaly detections for because of workload warm up
	// visibility: private
	// default_value: 180s
	AnomalyDetectionWorkloadWarmupPeriod time.Duration

	// description: AnomalyDetectionRateLimiterPeriod is the duration during which a limited number of anomaly detection events are allowed
	// visibility: private
	// default_value: 1m
	AnomalyDetectionRateLimiterPeriod time.Duration

	// description: AnomalyDetectionRateLimiterNumEventsAllowed is the number of anomaly detection events allowed per duration by the rate limiter
	// visibility: private
	// default_value: 10
	AnomalyDetectionRateLimiterNumEventsAllowed int

	// description: AnomalyDetectionRateLimiterNumKeys is the number of keys in the rate limiter
	// visibility: private
	// default_value: 256
	AnomalyDetectionRateLimiterNumKeys int

	// description: AnomalyDetectionTagRulesEnabled defines if the events that triggered anomaly detections should be tagged with the rules they might have matched.
	// visibility: private
	// default_value: true
	AnomalyDetectionTagRulesEnabled bool

	// description: AnomalyDetectionSilentRuleEventsEnabled do not send rule event if also part of an anomaly event
	// visibility: private
	// default_value: false
	AnomalyDetectionSilentRuleEventsEnabled bool

	// description: AnomalyDetectionEnabled defines if we should send anomaly detection events
	// visibility: private
	// default_value: true
	AnomalyDetectionEnabled bool

	// description: SBOMResolverEnabled defines if the SBOM resolver should be enabled
	// visibility: private
	// default_value: false
	SBOMResolverEnabled bool

	// description: SBOMResolverWorkloadsCacheSize defines the count of SBOMs to keep in memory in order to prevent re-computing the SBOMs of short-lived and periodical workloads
	// visibility: private
	// default_value: 10
	SBOMResolverWorkloadsCacheSize int

	// description: SBOMResolverHostEnabled defines if the SBOM resolver should compute the host's SBOM
	// visibility: private
	// default_value: false
	SBOMResolverHostEnabled bool

	// description: SBOMResolverEnrichmentInterval defines the minimum amount of time to wait before enriching an SBOM with runtime usage information
	// visibility: private
	// default_value: 1m
	SBOMResolverEnrichmentInterval time.Duration

	// description: SBOMResolverForwardInterval defines the interval for forwarding SBOMs
	// visibility: private
	// default_value: 20s
	SBOMResolverForwardInterval time.Duration

	// description: SBOMResolverRefreshInterval defines the interval for refreshing SBOMs
	// visibility: private
	// default_value: 3s
	SBOMResolverRefreshInterval time.Duration

	// description: SBOMResolverGeneratePolicies defines if the SBOM resolver should generate runtime security policies based on the computed SBOMs
	// visibility: private
	// default_value: false
	SBOMResolverGeneratePolicies bool

	// description: HashResolverEnabled defines if the hash resolver should be enabled
	// visibility: public
	// default_value: true
	HashResolverEnabled bool

	// description: HashResolverMaxFileSize defines the maximum size of the files that the hash resolver is allowed to hash
	// visibility: public
	// default_value: 5242880
	HashResolverMaxFileSize int64

	// description: HashResolverMaxHashRate defines the rate at which the hash resolver may compute hashes
	// visibility: public
	// default_value: 500
	HashResolverMaxHashRate int

	// description: HashResolverHashAlgorithms defines the hashes that hash resolver needs to compute
	// visibility: public
	// default_value: ["sha1", "sha256", "ssdeep"]
	HashResolverHashAlgorithms []model.HashAlgorithm

	// description: HashResolverEventTypes defines the list of event which files may be hashed
	// visibility: public
	// default_value: ["exec", "open"]
	HashResolverEventTypes []model.EventType

	// description: HashResolverCacheSize defines the number of hashes to keep in cache
	// visibility: public
	// default_value: 500
	HashResolverCacheSize int

	// description: HashResolverReplace is used to apply specific hash to specific file path
	// visibility: public
	// default_value: {}
	HashResolverReplace map[string]string

	// description: SysCtlEnabled defines if the sysctl event should be enabled
	// visibility: private
	// default_value: true
	SysCtlEnabled bool

	// description: SysCtlEBPFEnabled defines if the sysctl eBPF collection should be enabled
	// visibility: private
	// default_value: true
	SysCtlEBPFEnabled bool

	// description: SysCtlSnapshotEnabled defines if the sysctl snapshot feature should be enabled
	// visibility: private
	// default_value: true
	SysCtlSnapshotEnabled bool

	// description: SysCtlSnapshotPeriod defines at which time interval a new snapshot of sysctl parameters should be sent
	// visibility: private
	// default_value: 1h
	SysCtlSnapshotPeriod time.Duration

	// description: SysCtlSnapshotIgnoredBaseNames defines the list of basenaes that should be ignored from the snapshot
	// visibility: private
	// default_value: ["netdev_rss_key", "stable_secret"]
	SysCtlSnapshotIgnoredBaseNames []string

	// description: SysCtlSnapshotKernelCompilationFlags defines the list of kernel compilation flags that should be collected by the agent
	// visibility: private
	// default_value: {}
	SysCtlSnapshotKernelCompilationFlags map[string]uint8

	// description: UserSessionsCacheSize defines the size of the User Sessions cache size
	// visibility: private
	// default_value: 1024
	UserSessionsCacheSize int

	// description: SSHUserSessionsEnabled defines if SSH user session features should be enabled
	// visibility: public
	// default_value: true
	SSHUserSessionsEnabled bool

	// description: CaptureAllSyscallErrorsEnabled defines if the agent should capture all syscall errors
	// visibility: warning
	// default_value: false
	CaptureAllSyscallErrorsEnabled bool

	// description: EBPFLessEnabled enables the ebpfless probe
	// visibility: private
	// default_value: false
	EBPFLessEnabled bool

	// description: EBPFLessSocket defines the socket used for the communication between system-probe and the ebpfless source
	// visibility: private
	// default_value: localhost:5678
	EBPFLessSocket string

	// Enforcement capabilities
	// description: EnforcementEnabled defines if the enforcement capability should be enabled
	// visibility: private
	// default_value: true
	EnforcementEnabled bool

	// description: EnforcementRawSyscallEnabled defines if the enforcement should be performed using the sys_enter tracepoint
	// visibility: private
	// default_value: false
	EnforcementRawSyscallEnabled bool

	// description: EnforcementBinaryExcluded defines the list of binaries that are excluded from the enforcement
	// visibility: public
	// default_value: []
	EnforcementBinaryExcluded []string

	// description: EnforcementRuleSourceAllowed defines the list of rule sources that are allowed
	// visibility: public
	// default_value: ["file", "remote-config"]
	EnforcementRuleSourceAllowed []string

	// description: EnforcementDisarmerContainerEnabled defines if an enforcement rule should be disarmed when hitting too many different containers
	// visibility: private
	// default_value: true
	EnforcementDisarmerContainerEnabled bool

	// description: EnforcementDisarmerContainerMaxAllowed defines the maximum number of different containers that can trigger an enforcement rule within a period before the enforcement is disarmed for this rule
	// visibility: private
	// default_value: 5
	EnforcementDisarmerContainerMaxAllowed int

	// description: EnforcementDisarmerContainerPeriod defines the period during which EnforcementDisarmerContainerMaxAllowed is checked
	// visibility: private
	// default_value: 1m
	EnforcementDisarmerContainerPeriod time.Duration

	// description: EnforcementDisarmerExecutableEnabled defines if an enforcement rule should be disarmed when hitting too many different executables
	// visibility: private
	// default_value: true
	EnforcementDisarmerExecutableEnabled bool

	// description: EnforcementDisarmerExecutableMaxAllowed defines the maximum number of different executables that can trigger an enforcement rule within a period before the enforcement is disarmed for this rule
	// visibility: private
	// default_value: 5
	EnforcementDisarmerExecutableMaxAllowed int

	// description: EnforcementDisarmerExecutablePeriod defines the period during which EnforcementDisarmerExecutableMaxAllowed is checked
	// visibility: private
	// default_value: 1m
	EnforcementDisarmerExecutablePeriod time.Duration

	// description: WindowsFilenameCacheSize is the max number of filenames to cache
	// visibility: private
	// default_value: 16384
	WindowsFilenameCacheSize int

	// description: WindowsRegistryCacheSize is the max number of registry paths to cache
	// visibility: private
	// default_value: 4096
	WindowsRegistryCacheSize int

	// description: ETWEventsChannelSize windows specific ETW channel buffer size
	// visibility: private
	// default_value: 16384
	ETWEventsChannelSize int

	// description: ETWEventsMaxBuffers sets the maximumbuffers argument to ETW
	// visibility: private
	// default_value: 0
	ETWEventsMaxBuffers int

	// description: WindowsProbeChannelUnbuffered defines if the windows probe channel should be unbuffered
	// visibility: private
	// default_value: false
	WindowsProbeBlockOnChannelSend bool

	// description: WindowsProbeChannelUnbuffered defines if the windows probe channel should be unbuffered
	// visibility: private
	// default_value: 4096
	WindowsWriteEventRateLimiterMaxAllowed int

	// description: WindowsWriteEventRateLimiterPeriod defines the period during which WindowsWriteEventRateLimiterMaxAllowed is checked
	// visibility: private
	// default_value: 1s
	WindowsWriteEventRateLimiterPeriod time.Duration

	// description: IMDSIPv4 is used to provide a custom IP address for the IMDS endpoint
	// visibility: private
	// default_value: 169.254.169.254
	IMDSIPv4 uint32

	// description: EventGRPCServer defines which process should be used to send events and activity dumps
	// visibility: private
	// default_value: ""
	EventGRPCServer string

	// description: SendPayloadsFromSystemProbe defines when the event and activity dumps are sent directly from system-probe
	// visibility: private
	// default_value: false
	SendPayloadsFromSystemProbe bool

	// description: FileMetadataResolverEnabled defines if the file metadata is enabled
	// visibility: private
	// default_value: false
	FileMetadataResolverEnabled bool
}

// Config defines a security config
type Config struct {
	// Probe Config
	Probe *pconfig.Config

	// CWS specific parameters
	RuntimeSecurity *RuntimeSecurityConfig
}

// NewConfig returns a new Config object
func NewConfig() (*Config, error) {
	probeConfig, err := pconfig.NewConfig()
	if err != nil {
		return nil, err
	}

	rsConfig, err := NewRuntimeSecurityConfig()
	if err != nil {
		return nil, err
	}

	return &Config{
		Probe:           probeConfig,
		RuntimeSecurity: rsConfig,
	}, nil
}

// NewRuntimeSecurityConfig returns the runtime security (CWS) config, build from the system probe one
func NewRuntimeSecurityConfig() (*RuntimeSecurityConfig, error) {
	sysconfig.Adjust(pkgconfigsetup.SystemProbe())

	allEventTypes := make([]model.EventType, 0, model.MaxKernelEventType)

	var eventType model.EventType
	for i := uint64(0); i != uint64(model.MaxKernelEventType); i++ {
		eventType = model.EventType(i)
		allEventTypes = append(allEventTypes, eventType)
	}

	// parseEventTypeStringSlice converts a string list to a list of event types
	parseEventTypeStringSlice := func(eventTypes []string) []model.EventType {
		var output []model.EventType
		for _, eventTypeStr := range eventTypes {
			if eventTypeStr == "*" {
				return allEventTypes
			}
			eventType, err := model.ParseEvalEventType(eventTypeStr)
			if err != nil {
				seclog.Errorf("failed to parse event type '%s': %v", eventTypeStr, err)
				continue
			}
			if eventType == model.UnknownEventType {
				seclog.Errorf("unknown event type '%s'", eventTypeStr)
				continue
			}
			output = append(output, eventType)
		}
		return output
	}

	anomalyDetectionMinimumStablePeriods, err := parseEventTypeDurations(pkgconfigsetup.SystemProbe(), "runtime_security_config.security_profile.anomaly_detection.minimum_stable_period")
	if err != nil {
		return nil, err
	}

	rsConfig := &RuntimeSecurityConfig{
		RuntimeEnabled: pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.enabled"),
		FIMEnabled:     pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.fim_enabled"),

		// Windows specific
		WindowsFilenameCacheSize:               pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.windows_filename_cache_max"),
		WindowsRegistryCacheSize:               pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.windows_registry_cache_max"),
		ETWEventsChannelSize:                   pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.etw_events_channel_size"),
		WindowsProbeBlockOnChannelSend:         pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.windows_probe_block_on_channel_send"),
		WindowsWriteEventRateLimiterMaxAllowed: pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.windows_write_event_rate_limiter_max_allowed"),
		WindowsWriteEventRateLimiterPeriod:     pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.windows_write_event_rate_limiter_period"),

		SocketPath:               pkgconfigsetup.SystemProbe().GetString("runtime_security_config.socket"),
		CmdSocketPath:            pkgconfigsetup.SystemProbe().GetString("runtime_security_config.cmd_socket"),
		EventServerBurst:         pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.event_server.burst"),
		EventServerRate:          pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.event_server.rate"),
		EventServerRetention:     pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.event_server.retention"),
		EventRetryQueueThreshold: pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.event_retry_queue_threshold"),

		SelfTestEnabled:                 pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.self_test.enabled"),
		SelfTestSendReport:              pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.self_test.send_report"),
		RemoteConfigurationEnabled:      isRemoteConfigEnabled(),
		RemoteConfigurationDumpPolicies: pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.remote_configuration.dump_policies"),

		OnDemandEnabled:            pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.on_demand.enabled"),
		OnDemandRateLimiterEnabled: pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.on_demand.rate_limiter.enabled"),
		ReducedProcPidCacheSize:    pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.reduced_proc_pid_cache_size"),

		// policy & ruleset
		PoliciesDir:                         pkgconfigsetup.SystemProbe().GetString("runtime_security_config.policies.dir"),
		PolicyMonitorEnabled:                pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.policies.monitor.enabled"),
		PolicyMonitorPerRuleEnabled:         pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.policies.monitor.per_rule_enabled"),
		PolicyMonitorReportInternalPolicies: pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.policies.monitor.report_internal_policies"),
		RuleCacheEnabled:                    pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.policies.rule_cache_enabled"),

		LogPatterns: pkgconfigsetup.SystemProbe().GetStringSlice("runtime_security_config.log_patterns"),
		LogTags:     pkgconfigsetup.SystemProbe().GetStringSlice("runtime_security_config.log_tags"),
		EnvAsTags:   pkgconfigsetup.SystemProbe().GetStringSlice("runtime_security_config.env_as_tags"),

		// custom events
		InternalMonitoringEnabled: pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.internal_monitoring.enabled"),

		// activity dump
		ActivityDumpEnabled:                   pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.activity_dump.enabled"),
		ActivityDumpTraceSystemdCgroups:       pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.activity_dump.trace_systemd_cgroups"),
		ActivityDumpCleanupPeriod:             pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.activity_dump.cleanup_period"),
		ActivityDumpTagsResolutionPeriod:      pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.activity_dump.tags_resolution_period"),
		ActivityDumpLoadControlPeriod:         pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.activity_dump.load_controller_period"),
		ActivityDumpLoadControlMinDumpTimeout: pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.activity_dump.min_timeout"),
		ActivityDumpTracedCgroupsCount:        pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.activity_dump.traced_cgroups_count"),
		ActivityDumpTracedEventTypes:          parseEventTypeStringSlice(pkgconfigsetup.SystemProbe().GetStringSlice("runtime_security_config.activity_dump.traced_event_types")),
		ActivityDumpCgroupDumpTimeout:         pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.activity_dump.dump_duration"),
		ActivityDumpCgroupWaitListTimeout:     pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.activity_dump.cgroup_wait_list_timeout"),
		ActivityDumpCgroupDifferentiateArgs:   pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.activity_dump.cgroup_differentiate_args"),
		ActivityDumpLocalStorageDirectory:     pkgconfigsetup.SystemProbe().GetString("runtime_security_config.activity_dump.local_storage.output_directory"),
		ActivityDumpLocalStorageMaxDumpsCount: pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.activity_dump.local_storage.max_dumps_count"),
		ActivityDumpLocalStorageCompression:   pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.activity_dump.local_storage.compression"),
		ActivityDumpSyscallMonitorPeriod:      pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.activity_dump.syscall_monitor.period"),
		ActivityDumpMaxDumpCountPerWorkload:   pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.activity_dump.max_dump_count_per_workload"),
		ActivityDumpTagRulesEnabled:           pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.activity_dump.tag_rules.enabled"),
		ActivityDumpSilentWorkloadsDelay:      pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.activity_dump.silent_workloads.delay"),
		ActivityDumpSilentWorkloadsTicker:     pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.activity_dump.silent_workloads.ticker"),
		ActivityDumpWorkloadDenyList:          pkgconfigsetup.SystemProbe().GetStringSlice("runtime_security_config.activity_dump.workload_deny_list"),
		// activity dump dynamic fields
		ActivityDumpMaxDumpSize: func() int {
			mds := max(pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.activity_dump.max_dump_size"), ADMinMaxDumSize)
			return mds * (1 << 10)
		},

		// SBOM resolver
		SBOMResolverEnabled:            pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.sbom.enabled") || pkgconfigsetup.Datadog().GetBool("sbom.enrichment.usage.enabled"),
		SBOMResolverWorkloadsCacheSize: pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.sbom.workloads_cache_size"),
		SBOMResolverEnrichmentInterval: pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.sbom.enrichment_interval"),
		SBOMResolverRefreshInterval:    pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.sbom.refresh_interval"),
		SBOMResolverForwardInterval:    pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.sbom.forward_interval"),
		SBOMResolverHostEnabled:        pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.sbom.host.enabled"),
		SBOMResolverGeneratePolicies:   pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.sbom.generate_policies"),

		// Hash resolver
		HashResolverEnabled:        pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.hash_resolver.enabled"),
		HashResolverEventTypes:     parseEventTypeStringSlice(pkgconfigsetup.SystemProbe().GetStringSlice("runtime_security_config.hash_resolver.event_types")),
		HashResolverMaxFileSize:    pkgconfigsetup.SystemProbe().GetInt64("runtime_security_config.hash_resolver.max_file_size"),
		HashResolverHashAlgorithms: parseHashAlgorithmStringSlice(pkgconfigsetup.SystemProbe().GetStringSlice("runtime_security_config.hash_resolver.hash_algorithms")),
		HashResolverMaxHashRate:    pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.hash_resolver.max_hash_rate"),
		HashResolverCacheSize:      pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.hash_resolver.cache_size"),
		HashResolverReplace:        pkgconfigsetup.SystemProbe().GetStringMapString("runtime_security_config.hash_resolver.replace"),

		// SysCtl config parameter
		SysCtlEnabled:                        pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.sysctl.enabled"),
		SysCtlEBPFEnabled:                    pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.sysctl.ebpf.enabled"),
		SysCtlSnapshotEnabled:                pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.sysctl.snapshot.enabled"),
		SysCtlSnapshotPeriod:                 pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.sysctl.snapshot.period"),
		SysCtlSnapshotIgnoredBaseNames:       pkgconfigsetup.SystemProbe().GetStringSlice("runtime_security_config.sysctl.snapshot.ignored_base_names"),
		SysCtlSnapshotKernelCompilationFlags: map[string]uint8{},

		// event sampling (per-type)
		EventSamplingOpenEnabled:    pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.event_sampling.open.enabled"),
		EventSamplingOpenRate:       pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.event_sampling.open.rate"),
		EventSamplingConnectEnabled: pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.event_sampling.connect.enabled"),
		EventSamplingConnectRate:    pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.event_sampling.connect.rate"),
		EventSamplingBindEnabled:    pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.event_sampling.bind.enabled"),
		EventSamplingBindRate:       pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.event_sampling.bind.rate"),
		EventSamplingDNSEnabled:     pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.event_sampling.dns.enabled"),
		EventSamplingDNSRate:        pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.event_sampling.dns.rate"),

		// security profiles
		SecurityProfileEnabled:             pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.security_profile.enabled"),
		SecurityProfileV2Enabled:           pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.security_profile.v2.enabled"),
		SecurityProfileMaxImageTags:        pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.security_profile.max_image_tags"),
		SecurityProfileDir:                 pkgconfigsetup.SystemProbe().GetString("runtime_security_config.security_profile.dir"),
		SecurityProfileWatchDir:            pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.security_profile.watch_dir"),
		SecurityProfileCacheSize:           pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.security_profile.cache_size"),
		SecurityProfileMaxCount:            pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.security_profile.max_count"),
		SecurityProfileDNSMatchMaxDepth:    pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.security_profile.dns_match_max_depth"),
		SecurityProfileNodeEvictionTimeout: pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.security_profile.node_eviction_timeout"),
		SecurityProfileCleanupDelay:        pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.security_profile.profile_cleanup_delay"),
		SecurityProfileV2EventTypes:        parseEventTypeStringSlice(pkgconfigsetup.SystemProbe().GetStringSlice("runtime_security_config.security_profile.v2.event_types")),
		SecurityProfileSampleRefreshPeriod: pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.security_profile.v2.sample_refresh_period"),
		SecurityProfileV2ExcludedImages:    pkgconfigsetup.SystemProbe().GetStringSlice("runtime_security_config.security_profile.v2.excluded_images"),
		SecurityProfileV2MaxDumpSize: func() int {
			mds := max(pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.security_profile.v2.max_dump_size"), ADMinMaxDumSize)
			return mds * (1 << 10)
		},

		// anomaly detection
		AnomalyDetectionEventTypes:                   parseEventTypeStringSlice(pkgconfigsetup.SystemProbe().GetStringSlice("runtime_security_config.security_profile.anomaly_detection.event_types")),
		AnomalyDetectionDefaultMinimumStablePeriod:   pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.security_profile.anomaly_detection.default_minimum_stable_period"),
		AnomalyDetectionMinimumStablePeriods:         anomalyDetectionMinimumStablePeriods,
		AnomalyDetectionWorkloadWarmupPeriod:         pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.security_profile.anomaly_detection.workload_warmup_period"),
		AnomalyDetectionUnstableProfileTimeThreshold: pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.security_profile.anomaly_detection.unstable_profile_time_threshold"),
		AnomalyDetectionUnstableProfileSizeThreshold: pkgconfigsetup.SystemProbe().GetInt64("runtime_security_config.security_profile.anomaly_detection.unstable_profile_size_threshold"),
		AnomalyDetectionRateLimiterPeriod:            pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.security_profile.anomaly_detection.rate_limiter.period"),
		AnomalyDetectionRateLimiterNumKeys:           pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.security_profile.anomaly_detection.rate_limiter.num_keys"),
		AnomalyDetectionRateLimiterNumEventsAllowed:  pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.security_profile.anomaly_detection.rate_limiter.num_events_allowed"),
		AnomalyDetectionTagRulesEnabled:              pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.security_profile.anomaly_detection.tag_rules.enabled"),
		AnomalyDetectionSilentRuleEventsEnabled:      pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.security_profile.anomaly_detection.silent_rule_events.enabled"),
		AnomalyDetectionEnabled:                      pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.security_profile.anomaly_detection.enabled"),

		// enforcement
		EnforcementEnabled:                      pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.enforcement.enabled"),
		EnforcementBinaryExcluded:               pkgconfigsetup.SystemProbe().GetStringSlice("runtime_security_config.enforcement.exclude_binaries"),
		EnforcementRawSyscallEnabled:            pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.enforcement.raw_syscall.enabled"),
		EnforcementRuleSourceAllowed:            pkgconfigsetup.SystemProbe().GetStringSlice("runtime_security_config.enforcement.rule_source_allowed"),
		EnforcementDisarmerContainerEnabled:     pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.enforcement.disarmer.container.enabled"),
		EnforcementDisarmerContainerMaxAllowed:  pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.enforcement.disarmer.container.max_allowed"),
		EnforcementDisarmerContainerPeriod:      pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.enforcement.disarmer.container.period"),
		EnforcementDisarmerExecutableEnabled:    pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.enforcement.disarmer.executable.enabled"),
		EnforcementDisarmerExecutableMaxAllowed: pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.enforcement.disarmer.executable.max_allowed"),
		EnforcementDisarmerExecutablePeriod:     pkgconfigsetup.SystemProbe().GetDuration("runtime_security_config.enforcement.disarmer.executable.period"),

		// User Sessions
		SSHUserSessionsEnabled: pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.user_sessions.ssh.enabled"),
		UserSessionsCacheSize:  pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.user_sessions.cache_size"),

		// Capture all syscall errors
		CaptureAllSyscallErrorsEnabled: pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.syscalls.capture_all_errors.enabled"),

		// ebpf less
		EBPFLessEnabled: IsEBPFLessModeEnabled(),
		EBPFLessSocket:  pkgconfigsetup.SystemProbe().GetString("runtime_security_config.ebpfless.socket"),

		// IMDS
		IMDSIPv4: parseIMDSIPv4(),

		// event
		EventGRPCServer: pkgconfigsetup.SystemProbe().GetString("runtime_security_config.event_grpc_server"),

		// direct sender
		SendPayloadsFromSystemProbe: pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.direct_send_from_system_probe"),

		// FileMetadataResolverEnabled
		FileMetadataResolverEnabled: pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.file_metadata_resolver.enabled"),
	}

	compilationFlags := pkgconfigsetup.SystemProbe().GetStringSlice("runtime_security_config.sysctl.snapshot.kernel_compilation_flags")
	if len(compilationFlags) == 0 {
		compilationFlags = defaultKernelCompilationFlags
	}
	for _, configFlag := range compilationFlags {
		rsConfig.SysCtlSnapshotKernelCompilationFlags[configFlag] = 1
	}

	activityDumpRateLimiter := pkgconfigsetup.SystemProbe().GetInt("runtime_security_config.activity_dump.rate_limiter")
	if activityDumpRateLimiter < 0 || activityDumpRateLimiter > math.MaxUint16 {
		return nil, fmt.Errorf("invalid value for runtime_security_config.activity_dump.rate_limiter: %d, must be in uint16 range", activityDumpRateLimiter)
	}
	rsConfig.ActivityDumpRateLimiter = uint16(activityDumpRateLimiter)

	if rsConfig.SecurityProfileV2Enabled {
		rsConfig.EventSamplingOpenEnabled = true
		rsConfig.EventSamplingConnectEnabled = true
		rsConfig.EventSamplingBindEnabled = true
		rsConfig.EventSamplingDNSEnabled = true
	}

	if err := rsConfig.sanitize(); err != nil {
		return nil, err
	}

	return rsConfig, nil
}

// IsRuntimeEnabled returns true if any feature is enabled. Has to be applied in config package too
func (c *RuntimeSecurityConfig) IsRuntimeEnabled() bool {
	return c.RuntimeEnabled || c.FIMEnabled
}

// IsSysctlEventEnabled returns whether the sysctl event is enabled
func (c *RuntimeSecurityConfig) IsSysctlEventEnabled() bool {
	return c.SysCtlEnabled && c.SysCtlEBPFEnabled
}

// IsSysctlSnapshotEnabled returns whether the sysctl snapshot feature is enabled
func (c *RuntimeSecurityConfig) IsSysctlSnapshotEnabled() bool {
	return c.SysCtlEnabled && c.SysCtlSnapshotEnabled
}

// parseIMDSIPv4 returns the uint32 representation of the IMDS IP set by the configuration
func parseIMDSIPv4() uint32 {
	ip := pkgconfigsetup.SystemProbe().GetString("runtime_security_config.imds_ipv4")
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(parsedIP.To4())
}

// If RC is globally enabled, RC is enabled for CWS, unless the CWS-specific RC value is explicitly set to false
func isRemoteConfigEnabled() bool {
	// This value defaults to true
	rcEnabledInSysprobeConfig := pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.remote_configuration.enabled")

	if !rcEnabledInSysprobeConfig {
		return false
	}

	if configUtils.IsRemoteConfigEnabled(pkgconfigsetup.Datadog()) {
		return true
	}

	return false
}

// IsEBPFLessModeEnabled returns true if the ebpfless mode is enabled
// it's based on the configuration itself, but will default on true if
// running in sidecar mode
func IsEBPFLessModeEnabled() bool {
	const cfgKey = "runtime_security_config.ebpfless.enabled"
	// by default in sidecar mode, we enable ebpfless mode
	if !pkgconfigsetup.SystemProbe().IsConfigured(cfgKey) && fargate.IsSidecar() {
		seclog.Infof("Sidecar instance detected, enabling CWS ebpfless mode")
		pkgconfigsetup.SystemProbe().Set(cfgKey, true, pkgconfigmodel.SourceAgentRuntime)
	}

	return pkgconfigsetup.SystemProbe().GetBool(cfgKey)
}

// GetAnomalyDetectionMinimumStablePeriod returns the minimum stable period for a given event type
func (c *RuntimeSecurityConfig) GetAnomalyDetectionMinimumStablePeriod(eventType model.EventType) time.Duration {
	if minimumStablePeriod, found := c.AnomalyDetectionMinimumStablePeriods[eventType]; found {
		return minimumStablePeriod
	}
	return c.AnomalyDetectionDefaultMinimumStablePeriod
}

// sanitize ensures that the configuration is properly setup
func (c *RuntimeSecurityConfig) sanitize() error {
	serviceName := utils.GetTagValue("service", configUtils.GetConfiguredTags(pkgconfigsetup.Datadog(), true))
	if len(serviceName) > 0 {
		c.HostServiceName = serviceName
	}

	if c.IMDSIPv4 == 0 {
		return fmt.Errorf("invalid IPv4 address: got %v", pkgconfigsetup.SystemProbe().GetString("runtime_security_config.imds_ipv4"))
	}

	if c.EnforcementDisarmerContainerEnabled && c.EnforcementDisarmerContainerMaxAllowed <= 0 {
		return fmt.Errorf("invalid value for runtime_security_config.enforcement.disarmer.container.max_allowed: %d", c.EnforcementDisarmerContainerMaxAllowed)
	}

	if c.EnforcementDisarmerExecutableEnabled && c.EnforcementDisarmerExecutableMaxAllowed <= 0 {
		return fmt.Errorf("invalid value for runtime_security_config.enforcement.disarmer.executable.max_allowed: %d", c.EnforcementDisarmerExecutableMaxAllowed)
	}

	c.sanitizePlatform()

	return c.sanitizeRuntimeSecurityConfigActivityDump()
}

// sanitizeRuntimeSecurityConfigActivityDump ensures that runtime_security_config.activity_dump is properly configured
func (c *RuntimeSecurityConfig) sanitizeRuntimeSecurityConfigActivityDump() error {
	if !slices.Contains(c.ActivityDumpTracedEventTypes, model.ExecEventType) {
		c.ActivityDumpTracedEventTypes = append(c.ActivityDumpTracedEventTypes, model.ExecEventType)
	}

	if formats := pkgconfigsetup.SystemProbe().GetStringSlice("runtime_security_config.activity_dump.local_storage.formats"); len(formats) > 0 {
		var err error
		c.ActivityDumpLocalStorageFormats, err = ParseStorageFormats(formats)
		if err != nil {
			return fmt.Errorf("invalid value for runtime_security_config.activity_dump.local_storage.formats: %w", err)
		}
	}

	if c.ActivityDumpTracedCgroupsCount > model.MaxTracedCgroupsCount {
		c.ActivityDumpTracedCgroupsCount = model.MaxTracedCgroupsCount
	}

	if c.SecurityProfileEnabled && c.ActivityDumpEnabled && c.ActivityDumpLocalStorageDirectory != c.SecurityProfileDir {
		return fmt.Errorf("activity dumps storage directory '%s' has to be the same than security profile storage directory '%s'", c.ActivityDumpLocalStorageDirectory, c.SecurityProfileDir)
	}

	return nil
}

// parseEventTypeDurations converts a map of durations indexed by event types
func parseEventTypeDurations(cfg pkgconfigmodel.Config, prefix string) (map[model.EventType]time.Duration, error) {
	eventTypeMap := cfg.GetStringMap(prefix)
	eventTypeDurations := make(map[model.EventType]time.Duration, len(eventTypeMap))
	for eventTypeName := range eventTypeMap {
		eventType, err := model.ParseEvalEventType(eventTypeName)
		if err != nil {
			return nil, err
		}
		eventTypeDurations[eventType] = cfg.GetDuration(prefix + "." + eventTypeName)
	}
	return eventTypeDurations, nil
}

// parseHashAlgorithmStringSlice converts a string list to a list of hash algorithms
func parseHashAlgorithmStringSlice(algorithms []string) []model.HashAlgorithm {
	var output []model.HashAlgorithm
	for _, hashAlgorithm := range algorithms {
		for i := model.HashAlgorithm(0); i < model.MaxHashAlgorithm; i++ {
			if i.String() == hashAlgorithm {
				output = append(output, i)
				break
			}
		}
	}
	return output
}
