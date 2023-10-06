// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build podman

package podman

import (
	"net"
	"time"

	cnitypes "github.com/containernetworking/cni/pkg/types/100"
	"github.com/cri-o/ocicni/pkg/ocicni"
	spec "github.com/opencontainers/runtime-spec/specs-go"
)

// Note: This file includes the types from the Podman package that we need for
// our Podman DB client. The structs in this file have been copied from
// https://github.com/containers/podman/blob/v3.4.1/libpod/ with small
// modifications.
//
// More specifically, the types included here are:
// - ContainerStatus:
// https://github.com/containers/podman/blob/v3.4.1/libpod/define/containerstate.go
// - ContainerState (with only the attrs that we need):
// https://github.com/containers/podman/blob/v3.4.1/libpod/container.go
// - ContainerConfig (and the ones it depends on like ContainerSecurityConfig,
// ContainerNameSpaceConfig, etc.):
// https://github.com/containers/podman/blob/v3.4.1/libpod/container_config.go
//
// There are some modifications that have been done to avoid bringing
// unnecessary dependencies:
// - Deleted the `ContainerRootFSConfig` embedded struct from `ContainerConfig`.
// - Deleted the `IDMappings` field from `ContainerConfig`.
// - Deleted the `NetMode` field of the `ContainerNetworkConfig` struct.
// - Deleted the `HealthCheckConfig` field of the `ContainerMiscConfig` struct.
// - Deleted the `EnvSecrets` field of the `ContainerMiscConfig` struct.
// - The `Container` struct only contains the 2 attributes that we need.

// Container holds the configuration and the state of a container
type Container struct {
	Config *ContainerConfig
	State  *ContainerState
}

// ContainerStatus represents the current state of a container
type ContainerStatus int

const (
	// ContainerStateUnknown indicates that the container is in an error
	// state where information about it cannot be retrieved
	ContainerStateUnknown ContainerStatus = iota
	// ContainerStateConfigured indicates that the container has had its
	// storage configured but it has not been created in the OCI runtime
	ContainerStateConfigured ContainerStatus = iota
	// ContainerStateCreated indicates the container has been created in
	// the OCI runtime but not started
	ContainerStateCreated ContainerStatus = iota
	// ContainerStateRunning indicates the container is currently executing
	ContainerStateRunning ContainerStatus = iota
	// ContainerStateStopped indicates that the container was running but has
	// exited
	ContainerStateStopped ContainerStatus = iota
	// ContainerStatePaused indicates that the container has been paused
	ContainerStatePaused ContainerStatus = iota
	// ContainerStateExited indicates the the container has stopped and been
	// cleaned up
	ContainerStateExited ContainerStatus = iota
	// ContainerStateRemoving indicates the container is in the process of
	// being removed.
	ContainerStateRemoving ContainerStatus = iota
	// ContainerStateStopping indicates the container is in the process of
	// being stopped.
	ContainerStateStopping ContainerStatus = iota
)

// ContainerStatus returns a string representation for users
// of a container state
func (t ContainerStatus) String() string {
	switch t {
	case ContainerStateUnknown:
		return "unknown"
	case ContainerStateConfigured:
		return "configured"
	case ContainerStateCreated:
		return "created"
	case ContainerStateRunning:
		return "running"
	case ContainerStateStopped:
		return "stopped"
	case ContainerStatePaused:
		return "paused"
	case ContainerStateExited:
		return "exited"
	case ContainerStateRemoving:
		return "removing"
	case ContainerStateStopping:
		return "stopping"
	}
	return "bad state"
}

// ContainerState contains the current state of the container
// It is stored on disk in a tmpfs and recreated on reboot
type ContainerState struct {
	// The current state of the running container
	State ContainerStatus `json:"state"`
	// StartedTime is the time the container was started
	StartedTime time.Time `json:"startedTime,omitempty"`
	// FinishedTime is the time the container finished executing
	FinishedTime time.Time `json:"finishedTime,omitempty"`
	// PID is the PID of a running container
	PID int `json:"pid,omitempty"`
	// NetworkStatus contains the configuration results for all networks
	// the pod is attached to. Only populated if we created a network
	// namespace for the container, and the network namespace is currently
	// active
	NetworkStatus []*cnitypes.Result `json:"networkResults,omitempty"`
}

// ContainerConfig contains all information that was used to create the
// container. It may not be changed once created.
// It is stored, read-only, on disk in Libpod's State.
// Any changes will not be written back to the database, and will cause
// inconsistencies with other Libpod instances.
type ContainerConfig struct {
	// Spec is OCI runtime spec used to create the container. This is passed
	// in when the container is created, but it is not the final spec used
	// to run the container - it will be modified by Libpod to add things we
	// manage (e.g. bind mounts for /etc/resolv.conf, named volumes, a
	// network namespace prepared by CNI or slirp4netns) in the
	// generateSpec() function.
	Spec *spec.Spec `json:"spec"`

	// ID is a hex-encoded 256-bit pseudorandom integer used as a unique
	// identifier for the container. IDs are globally unique in Libpod -
	// once an ID is in use, no other container or pod will be created with
	// the same one until the holder of the ID has been removed.
	// ID is generated by Libpod, and cannot be chosen or influenced by the
	// user (except when restoring a checkpointed container).
	// ID is guaranteed to be 64 characters long.
	ID string `json:"id"`

	// Name is a human-readable name for the container. All containers must
	// have a non-empty name. Name may be provided when the container is
	// created; if no name is chosen, a name will be auto-generated.
	Name string `json:"name"`

	// Pod is the full ID of the pod the container belongs to. If the
	// container does not belong to a pod, this will be empty.
	// If this is not empty, a pod with this ID is guaranteed to exist in
	// the state for the duration of this container's existence.
	Pod string `json:"pod,omitempty"`

	// Namespace is the libpod Namespace the container is in.
	// Namespaces are used to divide containers in the state.
	Namespace string `json:"namespace,omitempty"`

	// LockID is the ID of this container's lock. Each container, pod, and
	// volume is assigned a unique Lock (from one of several backends) by
	// the libpod Runtime. This lock will belong only to this container for
	// the duration of the container's lifetime.
	LockID uint32 `json:"lockID"`

	// CreateCommand is the full command plus arguments that were used to
	// create the container. It is shown in the output of Inspect, and may
	// be used to recreate an identical container for automatic updates or
	// portable systemd unit files.
	CreateCommand []string `json:"CreateCommand,omitempty"`

	// RawImageName is the raw and unprocessed name of the image when creating
	// the container (as specified by the user).  May or may not be set.  One
	// use case to store this data are auto-updates where we need the _exact_
	// name and not some normalized instance of it.
	RawImageName string `json:"RawImageName,omitempty"`

	// IDMappings are UID/GID mappings used by the container's user
	// namespace. They are used by the OCI runtime when creating the
	// container, and by c/storage to ensure that the container's files have
	// the appropriate owner.
	//IDMappings storage.IDMappingOptions `json:"idMappingsOptions,omitempty"`

	// Dependencies are the IDs of dependency containers.
	// These containers must be started before this container is started.
	Dependencies []string

	// embedded sub-configs
	ContainerRootFSConfig
	ContainerSecurityConfig
	ContainerNameSpaceConfig
	ContainerNetworkConfig
	ContainerImageConfig
	ContainerMiscConfig
}

// ContainerRootFSConfig is an embedded sub-config providing config info about the container's root fs.
// We use it to get the container's imageID
type ContainerRootFSConfig struct {
	// RootfsImageID is the ID of the image used to create the container.
	// If the container was created from a Rootfs, this will be empty.
	// If non-empty, Podman will create a root filesystem for the container
	// based on an image with this ID.
	// This conflicts with Rootfs.
	RootfsImageID string `json:"rootfsImageID,omitempty"`
}

// ContainerSecurityConfig is an embedded sub-config providing security configuration
// to the container.
type ContainerSecurityConfig struct {
	// Privileged is whether the container is privileged. Privileged
	// containers have lessened security and increased access to the system.
	// Note that this does NOT directly correspond to Podman's --privileged
	// flag - most of the work of that flag is done in creating the OCI spec
	// given to Libpod. This only enables a small subset of the overall
	// operation, mostly around mounting the container image with reduced
	// security.
	Privileged bool `json:"privileged"`
	// ProcessLabel is the SELinux process label for the container.
	ProcessLabel string `json:"ProcessLabel,omitempty"`
	// MountLabel is the SELinux mount label for the container's root
	// filesystem. Only used if the container was created from an image.
	// If not explicitly set, an unused random MLS label will be assigned by
	// containers/storage (but only if SELinux is enabled).
	MountLabel string `json:"MountLabel,omitempty"`
	// LabelOpts are options passed in by the user to setup SELinux labels.
	// These are used by the containers/storage library.
	LabelOpts []string `json:"labelopts,omitempty"`
	// User and group to use in the container. Can be specified as only user
	// (in which case we will attempt to look up the user in the container
	// to determine the appropriate group) or user and group separated by a
	// colon.
	// Can be specified by name or UID/GID.
	// If unset, this will default to UID and GID 0 (root).
	User string `json:"user,omitempty"`
	// Groups are additional groups to add the container's user to. These
	// are resolved within the container using the container's /etc/passwd.
	Groups []string `json:"groups,omitempty"`
	// AddCurrentUserPasswdEntry indicates that Libpod should ensure that
	// the container's /etc/passwd contains an entry for the user running
	// Libpod - mostly used in rootless containers where the user running
	// Libpod wants to retain their UID inside the container.
	AddCurrentUserPasswdEntry bool `json:"addCurrentUserPasswdEntry,omitempty"`
}

// ContainerNameSpaceConfig is an embedded sub-config providing
// namespace configuration to the container.
type ContainerNameSpaceConfig struct {
	// IDs of container to share namespaces with
	// NetNsCtr conflicts with the CreateNetNS bool
	// These containers are considered dependencies of the given container
	// They must be started before the given container is started
	IPCNsCtr    string `json:"ipcNsCtr,omitempty"`
	MountNsCtr  string `json:"mountNsCtr,omitempty"`
	NetNsCtr    string `json:"netNsCtr,omitempty"`
	PIDNsCtr    string `json:"pidNsCtr,omitempty"`
	UserNsCtr   string `json:"userNsCtr,omitempty"`
	UTSNsCtr    string `json:"utsNsCtr,omitempty"`
	CgroupNsCtr string `json:"cgroupNsCtr,omitempty"`
}

// ContainerNetworkConfig is an embedded sub-config providing network configuration
// to the container.
type ContainerNetworkConfig struct {
	// CreateNetNS indicates that libpod should create and configure a new
	// network namespace for the container.
	// This cannot be set if NetNsCtr is also set.
	CreateNetNS bool `json:"createNetNS"`
	// StaticIP is a static IP to request for the container.
	// This cannot be set unless CreateNetNS is set.
	// If not set, the container will be dynamically assigned an IP by CNI.
	StaticIP net.IP `json:"staticIP"`
	// StaticMAC is a static MAC to request for the container.
	// This cannot be set unless CreateNetNS is set.
	// If not set, the container will be dynamically assigned a MAC by CNI.
	StaticMAC net.HardwareAddr `json:"staticMAC"`
	// PortMappings are the ports forwarded to the container's network
	// namespace
	// These are not used unless CreateNetNS is true
	PortMappings []ocicni.PortMapping `json:"portMappings,omitempty"`
	// ExposedPorts are the ports which are exposed but not forwarded
	// into the container.
	// The map key is the port and the string slice contains the protocols,
	// e.g. tcp and udp
	// These are only set when exposed ports are given but not published.
	ExposedPorts map[uint16][]string `json:"exposedPorts,omitempty"`
	// UseImageResolvConf indicates that resolv.conf should not be
	// bind-mounted inside the container.
	// Conflicts with DNSServer, DNSSearch, DNSOption.
	UseImageResolvConf bool
	// DNS servers to use in container resolv.conf
	// Will override servers in host resolv if set
	DNSServer []net.IP `json:"dnsServer,omitempty"`
	// DNS Search domains to use in container resolv.conf
	// Will override search domains in host resolv if set
	DNSSearch []string `json:"dnsSearch,omitempty"`
	// DNS options to be set in container resolv.conf
	// With override options in host resolv if set
	DNSOption []string `json:"dnsOption,omitempty"`
	// UseImageHosts indicates that /etc/hosts should not be
	// bind-mounted inside the container.
	// Conflicts with HostAdd.
	UseImageHosts bool
	// Hosts to add in container
	// Will be appended to host's host file
	HostAdd []string `json:"hostsAdd,omitempty"`
	// Network names (CNI) to add container to. Empty to use default network.
	// Please note that these can be altered at runtime. The actual list is
	// stored in the DB and should be retrieved from there; this is only the
	// set of networks the container was *created* with.
	Networks []string `json:"networks,omitempty"`
	// Network mode specified for the default network.
	// NetMode namespaces.NetworkMode `json:"networkMode,omitempty"`
	// NetworkOptions are additional options for each network
	NetworkOptions map[string][]string `json:"network_options,omitempty"`
	// NetworkAliases are aliases that will be added to each network.
	// These are additional names that this container can be accessed as via
	// DNS when the CNI dnsname plugin is in use.
	// Please note that these can be altered at runtime. As such, the actual
	// list is stored in the database and should be retrieved from there;
	// this is only the set of aliases the container was *created with*.
	// Formatted as map of network name to aliases. All network names must
	// be present in the Networks list above.
	NetworkAliases map[string][]string `json:"network_alises,omitempty"`
}

// ContainerImageConfig is an embedded sub-config providing image configuration
// to the container.
type ContainerImageConfig struct {
	// UserVolumes contains user-added volume mounts in the container.
	// These will not be added to the container's spec, as it is assumed
	// they are already present in the spec given to Libpod. Instead, it is
	// used when committing containers to generate the VOLUMES field of the
	// image that is created, and for triggering some OCI hooks which do not
	// fire unless user-added volume mounts are present.
	UserVolumes []string `json:"userVolumes,omitempty"`
	// Entrypoint is the container's entrypoint.
	// It is not used in spec generation, but will be used when the
	// container is committed to populate the entrypoint of the new image.
	Entrypoint []string `json:"entrypoint,omitempty"`
	// Command is the container's command.
	// It is not used in spec generation, but will be used when the
	// container is committed to populate the command of the new image.
	Command []string `json:"command,omitempty"`
}

// ContainerMiscConfig is an embedded sub-config providing misc configuration
// to the container.
type ContainerMiscConfig struct {
	// Whether to keep container STDIN open
	Stdin bool `json:"stdin,omitempty"`
	// Labels is a set of key-value pairs providing additional information
	// about a container
	Labels map[string]string `json:"labels,omitempty"`
	// StopSignal is the signal that will be used to stop the container
	StopSignal uint `json:"stopSignal,omitempty"`
	// StopTimeout is the signal that will be used to stop the container
	StopTimeout uint `json:"stopTimeout,omitempty"`
	// Timeout is maximum time a container will run before getting the kill signal
	Timeout uint `json:"timeout,omitempty"`
	// Time container was created
	CreatedTime time.Time `json:"createdTime"`
	// CgroupManager is the cgroup manager used to create this container.
	// If empty, the runtime default will be used.
	CgroupManager string `json:"cgroupManager,omitempty"`
	// NoCgroups indicates that the container will not create CGroups. It is
	// incompatible with CgroupParent.  Deprecated in favor of CgroupsMode.
	NoCgroups bool `json:"noCgroups,omitempty"`
	// CgroupsMode indicates how the container will create cgroups
	// (disabled, no-conmon, enabled).  It supersedes NoCgroups.
	CgroupsMode string `json:"cgroupsMode,omitempty"`
	// Cgroup parent of the container.
	CgroupParent string `json:"cgroupParent"`
	// LogPath log location
	LogPath string `json:"logPath"`
	// LogTag is the tag used for logging
	LogTag string `json:"logTag"`
	// LogSize is the tag used for logging
	LogSize int64 `json:"logSize"`
	// LogDriver driver for logs
	LogDriver string `json:"logDriver"`
	// File containing the conmon PID
	ConmonPidFile string `json:"conmonPidFile,omitempty"`
	// RestartPolicy indicates what action the container will take upon
	// exiting naturally.
	// Allowed options are "no" (take no action), "on-failure" (restart on
	// non-zero exit code, up an a maximum of RestartRetries times),
	// and "always" (always restart the container on any exit code).
	// The empty string is treated as the default ("no")
	RestartPolicy string `json:"restart_policy,omitempty"`
	// RestartRetries indicates the number of attempts that will be made to
	// restart the container. Used only if RestartPolicy is set to
	// "on-failure".
	RestartRetries uint `json:"restart_retries,omitempty"`
	// TODO log options for log drivers
	// PostConfigureNetNS needed when a user namespace is created by an OCI runtime
	// if the network namespace is created before the user namespace it will be
	// owned by the wrong user namespace.
	PostConfigureNetNS bool `json:"postConfigureNetNS"`
	// OCIRuntime used to create the container
	OCIRuntime string `json:"runtime,omitempty"`
	// ExitCommand is the container's exit command.
	// This Command will be executed when the container exits by Conmon.
	// It is usually used to invoke post-run cleanup - for example, in
	// Podman, it invokes `podman container cleanup`, which in turn calls
	// Libpod's Cleanup() API to unmount the container and clean up its
	// network.
	ExitCommand []string `json:"exitCommand,omitempty"`
	// IsInfra is a bool indicating whether this container is an infra container used for
	// sharing kernel namespaces in a pod
	IsInfra bool `json:"pause"`
	// SdNotifyMode tells libpod what to do with a NOTIFY_SOCKET if passed
	SdNotifyMode string `json:"sdnotifyMode,omitempty"`
	// Systemd tells libpod to setup the container in systemd mode
	Systemd bool `json:"systemd"`
	// HealthCheckConfig has the health check command and related timings
	//HealthCheckConfig *manifest.Schema2HealthConfig `json:"healthcheck"`
	// PreserveFDs is a number of additional file descriptors (in addition
	// to 0, 1, 2) that will be passed to the executed process. The total FDs
	// passed will be 3 + PreserveFDs.
	PreserveFDs uint `json:"preserveFds,omitempty"`
	// Timezone is the timezone inside the container.
	// Local means it has the same timezone as the host machine
	Timezone string `json:"timezone,omitempty"`
	// Umask is the umask inside the container.
	Umask string `json:"umask,omitempty"`
	// PidFile is the file that saves the pid of the container process
	PidFile string `json:"pid_file,omitempty"`
	// CDIDevices contains devices that use the CDI
	CDIDevices []string `json:"cdiDevices,omitempty"`
	// EnvSecrets are secrets that are set as environment variables
	//EnvSecrets map[string]*secrets.Secret `json:"secret_env,omitempty"`
	// InitContainerType specifies if the container is an initcontainer
	// and if so, what type: always or once are possible non-nil entries
	InitContainerType string `json:"init_container_type,omitempty"`
}
