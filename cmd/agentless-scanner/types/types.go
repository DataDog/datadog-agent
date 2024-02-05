// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package types holds the different types and constants used in the agentless
// scanner. Most of these types are serializable.
package types

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	cdx "github.com/CycloneDX/cyclonedx-go"
	sbommodel "github.com/DataDog/agent-payload/v5/sbom"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/docker/distribution/reference"
)

const (
	// ScansRootDir is the root directory where scan tasks can store state on disk.
	ScansRootDir = "/scans"

	// EBSMountPrefix is the prefix for EBS mounts.
	EBSMountPrefix = "ebs-"
	// ContainerdMountPrefix is the prefix for containerd overlay mounts.
	ContainerdMountPrefix = "containerd-"
	// DockerMountPrefix is the prefix for Docker overlay mounts.
	DockerMountPrefix = "docker-"
	// LambdaMountPrefix is the prefix for Lambda code data.
	LambdaMountPrefix = "lambda-"
)

// ConfigType is the type of the scan configuration
type ConfigType string

const (
	// ConfigTypeAWS is the type of the scan configuration for AWS
	ConfigTypeAWS ConfigType = "aws-scan"
)

// TaskType is the type of the scan
type TaskType string

const (
	// TaskTypeHost is the type of the scan for a host
	TaskTypeHost TaskType = "localhost-scan"
	// TaskTypeEBS is the type of the scan for an EBS volume
	TaskTypeEBS TaskType = "ebs-volume"
	// TaskTypeLambda is the type of the scan for a Lambda function
	TaskTypeLambda TaskType = "lambda"
)

// ScanAction is the action to perform during the scan
type ScanAction string

const (
	// ScanActionMalware is the action to scan for malware
	ScanActionMalware ScanAction = "malware"
	// ScanActionVulnsHost is the action to scan for vulnerabilities on hosts
	ScanActionVulnsHost ScanAction = "vulns"
	// ScanActionVulnsContainers is the action to scan for vulnerabilities in containers
	ScanActionVulnsContainers ScanAction = "vulnscontainers"
)

// DiskMode is the mode to attach the disk
type DiskMode string

const (
	// DiskModeVolumeAttach is the mode to attach the disk as a volume
	DiskModeVolumeAttach DiskMode = "volume-attach"
	// DiskModeNBDAttach is the mode to attach the disk as a NBD
	DiskModeNBDAttach DiskMode = "nbd-attach"
	// DiskModeNoAttach is the mode to not attach the disk (using user-space filesystem drivers)
	DiskModeNoAttach = "no-attach"
)

// ScannerName is the name of the scanner
type ScannerName string

const (
	// ScannerNameHostVulns is the name of the scanner for host vulnerabilities
	ScannerNameHostVulns ScannerName = "hostvulns"
	// ScannerNameHostVulnsVM is the name of the scanner for host vulnerabilities (using userspace drivers for filesystems)
	ScannerNameHostVulnsVM ScannerName = "hostvulns-vm"
	// ScannerNameContainerVulns is the name of the scanner for container vulnerabilities
	ScannerNameContainerVulns ScannerName = "containervulns"
	// ScannerNameAppVulns is the name of the scanner for application vulnerabilities
	ScannerNameAppVulns ScannerName = "appvulns"
	// ScannerNameContainers is the name of the scanner for containers
	ScannerNameContainers ScannerName = "containers"
	// ScannerNameMalware is the name of the scanner for malware
	ScannerNameMalware ScannerName = "malware"
)

// ResourceType is the type of the cloud resource
type ResourceType string

const (
	// ResourceTypeLocalDir is the type of a local directory
	ResourceTypeLocalDir = "localdir"
	// ResourceTypeVolume is the type of a volume
	ResourceTypeVolume = "volume"
	// ResourceTypeSnapshot is the type of a snapshot
	ResourceTypeSnapshot = "snapshot"
	// ResourceTypeFunction is the type of a function
	ResourceTypeFunction = "function"
	// ResourceTypeRole is the type of a role
	ResourceTypeRole = "role"
)

// RolesMapping is the mapping of roles from accounts IDs to role IDs
type RolesMapping map[string]*CloudID

// ScanConfigRaw is the raw representation of the scan configuration received
// from RC.
type ScanConfigRaw struct {
	Type  string `json:"type"`
	Tasks []struct {
		Type     string   `json:"type"`
		CloudID  string   `json:"arn"`
		Hostname string   `json:"hostname"`
		Actions  []string `json:"actions,omitempty"`
		DiskMode string   `json:"disk_mode,omitempty"`
	} `json:"tasks"`
	Roles []string `json:"roles"`
}

// ScanConfig is the representation of the scan configuration after being
// parsed and normalized.
type ScanConfig struct {
	Type  ConfigType
	Tasks []*ScanTask
	Roles RolesMapping
}

// ScanTask is the representation of a scan task that performs a scan on a
// resource.
type ScanTask struct {
	ID              string       `json:"ID"`
	CreatedAt       time.Time    `json:"CreatedAt"`
	StartedAt       time.Time    `json:"StartedAt"`
	Type            TaskType     `json:"Type"`
	CloudID         CloudID      `json:"CloudID"`
	TargetHostname  string       `json:"Hostname"`
	ScannerHostname string       `json:"ScannerHostname"`
	Actions         []ScanAction `json:"Actions"`
	Roles           RolesMapping `json:"Roles"`
	DiskMode        DiskMode     `json:"DiskMode"`

	// Lifecycle metadata of the task
	CreatedResources   map[CloudID]time.Time `json:"CreatedResources"`
	AttachedDeviceName *string               `json:"AttachedDeviceName"`
}

// Path returns the path to the scan task. It takes a list of names to join.
func (s *ScanTask) Path(names ...string) string {
	root := filepath.Join(ScansRootDir, s.ID)
	for _, name := range names {
		name = strings.ToLower(name)
		name = regexp.MustCompile("[^a-z0-9_.-]").ReplaceAllString(name, "")
		root = filepath.Join(root, name)
	}
	return root
}

// PushCreatedResource adds a created resource to the scan task. This is used
// to track of resources that require to be cleaned up at the end of the scan.
func (s *ScanTask) PushCreatedResource(resourceID CloudID, createdAt time.Time) {
	s.CreatedResources[resourceID] = createdAt
}

func (s *ScanTask) String() string {
	if s == nil {
		return "nilscan"
	}
	return s.ID
}

// Tags returns the tags for the scan task.
func (s *ScanTask) Tags(rest ...string) []string {
	return append([]string{
		fmt.Sprintf("agent_version:%s", version.AgentVersion),
		fmt.Sprintf("region:%s", s.CloudID.Region),
		fmt.Sprintf("type:%s", s.Type),
	}, rest...)
}

// TagsNoResult returns the tags for a scan task with no result.
func (s *ScanTask) TagsNoResult() []string {
	return s.Tags("status:noresult")
}

// TagsNotFound returns the tags for a scan task with not found resource.
func (s *ScanTask) TagsNotFound() []string {
	return s.Tags("status:notfound")
}

// TagsFailure returns the tags for a scan task that failed.
func (s *ScanTask) TagsFailure(err error) []string {
	if errors.Is(err, context.Canceled) {
		return s.Tags("status:canceled")
	}
	return s.Tags("status:failure")
}

// TagsSuccess returns the tags for a scan task that succeeded.
func (s *ScanTask) TagsSuccess() []string {
	return s.Tags("status:success")
}

// ScanJSONError is a wrapper for errors that can be marshaled and unmarshaled.
type ScanJSONError struct {
	err error
}

// Error implements the error interface.
func (e *ScanJSONError) Error() string {
	return e.err.Error()
}

// Unwrap implements the errors.Wrapper interface.
func (e *ScanJSONError) Unwrap() error {
	return e.err
}

// MarshalJSON implements the json.Marshaler interface.
func (e *ScanJSONError) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.err.Error())
}

// UnmarshalJSON implements the json.Marshaler interface.
func (e *ScanJSONError) UnmarshalJSON(data []byte) error {
	var msg string
	if err := json.Unmarshal(data, &msg); err != nil {
		return err
	}
	e.err = errors.New(msg)
	return nil
}

// ScanResult is the result of a scan task. A scan task can emit multiple results.
type ScanResult struct {
	ScannerOptions

	Err *ScanJSONError `json:"Err"`

	// Results union
	Vulns      *ScanVulnsResult     `json:"Vulns"`
	Malware    *ScanMalwareResult   `json:"Malware"`
	Containers *ScanContainerResult `json:"Containers"`
}

// ScannerOptions is the options to configure a scanner.
type ScannerOptions struct {
	Scanner   ScannerName `json:"Scanner"`
	Scan      *ScanTask   `json:"Scan"`
	Root      string      `json:"Root"`
	CreatedAt time.Time   `jons:"CreatedAt"`
	StartedAt time.Time   `jons:"StartedAt"`
	Container *Container  `json:"Container"`

	// Used for ScannerNameHostVulnsVM
	SnapshotID *CloudID `json:"SnapshotID"`
}

// ErrResult returns a ScanResult with an error.
func (o ScannerOptions) ErrResult(err error) ScanResult {
	return ScanResult{ScannerOptions: o, Err: &ScanJSONError{err}}
}

// ID returns the ID of the scanner options.
func (o ScannerOptions) ID() string {
	h := sha256.New()
	createdAt, _ := o.CreatedAt.MarshalBinary()
	h.Write(createdAt)
	h.Write([]byte(o.Scanner))
	h.Write([]byte(o.Root))
	h.Write([]byte(o.Scan.ID))
	if ctr := o.Container; ctr != nil {
		h.Write([]byte((*ctr).String()))
	}
	return string(o.Scanner) + "-" + hex.EncodeToString(h.Sum(nil)[:8])
}

// ScanVulnsResult is the result of a vulnerability scan.
type ScanVulnsResult struct {
	BOM        *cdx.BOM                 `json:"BOM"`
	SourceType sbommodel.SBOMSourceType `json:"SourceType"`
	ID         string                   `json:"ID"`
	Tags       []string                 `json:"Tags"`
}

// ScanContainerResult is the result of a container scan.
type ScanContainerResult struct {
	Containers []*Container `json:"Containers"`
}

// ScanMalwareResult is the result of a malware scan.
type ScanMalwareResult struct {
	Findings []*ScanFinding
}

// ScanFinding is a finding from a scan.
type ScanFinding struct {
	AgentVersion string      `json:"agent_version,omitempty"`
	RuleID       string      `json:"agent_rule_id,omitempty"`
	RuleVersion  int         `json:"agent_rule_version,omitempty"`
	FrameworkID  string      `json:"agent_framework_id,omitempty"`
	Evaluator    string      `json:"evaluator,omitempty"`
	ExpireAt     *time.Time  `json:"expire_at,omitempty"`
	Result       string      `json:"result,omitempty"`
	ResourceType string      `json:"resource_type,omitempty"`
	ResourceID   string      `json:"resource_id,omitempty"`
	Tags         []string    `json:"tags"`
	Data         interface{} `json:"data"`
}

// Container is the representation of a container.
type Container struct {
	Runtime           string          `json:"Runtime"`
	MountName         string          `json:"MountName"`
	ImageRefTagged    reference.Field `json:"ImageRefTagged"`    // public.ecr.aws/datadog/agent:7-rc
	ImageRefCanonical reference.Field `json:"ImageRefCanonical"` // public.ecr.aws/datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409
	ContainerName     string          `json:"ContainerName"`
	Layers            []string        `json:"Layers"`
}

func (c Container) String() string {
	return fmt.Sprintf("%s/%s/%s", c.Runtime, c.ContainerName, c.ImageRefCanonical.Reference())
}

// ParseTaskType parses a scan type from a string.
func ParseTaskType(scanType string) (TaskType, error) {
	switch scanType {
	case string(TaskTypeHost):
		return TaskTypeHost, nil
	case string(TaskTypeEBS):
		return TaskTypeEBS, nil
	case string(TaskTypeLambda):
		return TaskTypeLambda, nil
	default:
		return "", fmt.Errorf("unknown scan type %q", scanType)
	}
}

// DefaultTaskType returns the default scan type for a resource.
func DefaultTaskType(resourceID CloudID) (TaskType, error) {
	resourceType := resourceID.ResourceType()
	switch resourceType {
	case ResourceTypeLocalDir:
		return TaskTypeHost, nil
	case ResourceTypeSnapshot, ResourceTypeVolume:
		return TaskTypeEBS, nil
	case ResourceTypeFunction:
		return TaskTypeLambda, nil
	default:
		return "", fmt.Errorf("invalid resource type %q for scanning, expecting %s, %s or %s", resourceType, ResourceTypeLocalDir, ResourceTypeVolume, ResourceTypeSnapshot)
	}
}

// ParseDiskMode parses a disk mode from a string.
func ParseDiskMode(diskMode string) (DiskMode, error) {
	switch diskMode {
	case string(DiskModeNoAttach):
		return DiskModeNoAttach, nil
	case string(DiskModeVolumeAttach):
		return DiskModeVolumeAttach, nil
	case string(DiskModeNBDAttach), "":
		return DiskModeNBDAttach, nil
	default:
		return "", fmt.Errorf("invalid disk mode %q, expecting either %s, %s or %s", diskMode, DiskModeVolumeAttach, DiskModeNBDAttach, DiskModeNoAttach)
	}
}

// ParseRolesMapping parses a list of roles into a mapping from account ID to
// role cloud resource ID.
func ParseRolesMapping(roles []string) RolesMapping {
	if len(roles) == 0 {
		return nil
	}
	rolesMap := make(RolesMapping, len(roles))
	for _, role := range roles {
		roleID, err := ParseCloudID(role, ResourceTypeRole)
		if err != nil {
			continue
		}
		rolesMap[roleID.AccountID] = &roleID
	}
	return rolesMap
}

// NewScanTask creates a new scan task.
func NewScanTask(taskType TaskType, resourceID, scannerHostname, targetHostname string, actions []ScanAction, roles RolesMapping, mode DiskMode) (*ScanTask, error) {
	var cloudID CloudID
	var err error
	switch taskType {
	case TaskTypeEBS:
		cloudID, err = ParseCloudID(resourceID, ResourceTypeSnapshot, ResourceTypeVolume)
	case TaskTypeHost:
		cloudID, err = ParseCloudID(resourceID, ResourceTypeLocalDir)
	case TaskTypeLambda:
		cloudID, err = ParseCloudID(resourceID, ResourceTypeFunction)
	default:
		err = fmt.Errorf("unsupported task type %q", taskType)
	}
	if err != nil {
		return nil, err
	}
	task := ScanTask{
		Type:             taskType,
		CloudID:          cloudID,
		TargetHostname:   targetHostname,
		ScannerHostname:  scannerHostname,
		Roles:            roles,
		DiskMode:         mode,
		Actions:          actions,
		CreatedAt:        time.Now(),
		CreatedResources: make(map[CloudID]time.Time),
	}
	{
		h := sha256.New()
		createdAt, _ := task.CreatedAt.MarshalBinary()
		h.Write(createdAt)
		h.Write([]byte(task.Type))
		h.Write([]byte(task.CloudID.String()))
		h.Write([]byte(task.TargetHostname))
		h.Write([]byte(task.ScannerHostname))
		h.Write([]byte(task.DiskMode))
		for _, action := range task.Actions {
			h.Write([]byte(action))
		}
		task.ID = string(task.Type) + "-" + hex.EncodeToString(h.Sum(nil)[:8])
	}
	return &task, nil
}

// ParseScanAction parses a scan action from a string.
func ParseScanAction(action string) (ScanAction, error) {
	switch action {
	case string(ScanActionVulnsHost):
		return ScanActionVulnsHost, nil
	case string(ScanActionVulnsContainers):
		return ScanActionVulnsContainers, nil
	case string(ScanActionMalware):
		return ScanActionMalware, nil
	default:
		return "", fmt.Errorf("unknown action type %q", action)
	}
}

// ParseScanActions parses a list of actions as strings into a list of scan actions.
func ParseScanActions(actions []string) ([]ScanAction, error) {
	var scanActions []ScanAction
	for _, a := range actions {
		action, err := ParseScanAction(a)
		if err != nil {
			return nil, err
		}
		scanActions = append(scanActions, action)
	}
	return scanActions, nil
}

// UnmarshalConfig unmarshals a scan configuration from a JSON byte slice.
func UnmarshalConfig(b []byte, scannerHostname string, defaultActions []ScanAction, defaultRolesMapping RolesMapping) (*ScanConfig, error) {
	var configRaw ScanConfigRaw
	err := json.Unmarshal(b, &configRaw)
	if err != nil {
		return nil, err
	}

	var config ScanConfig
	switch configRaw.Type {
	case string(ConfigTypeAWS):
		config.Type = ConfigTypeAWS
	default:
		return nil, fmt.Errorf("unexpected config type %q", config.Type)
	}

	if len(configRaw.Roles) > 0 {
		config.Roles = ParseRolesMapping(configRaw.Roles)
	} else {
		config.Roles = defaultRolesMapping
	}

	config.Tasks = make([]*ScanTask, 0, len(configRaw.Tasks))
	for _, rawScan := range configRaw.Tasks {
		var actions []ScanAction
		if rawScan.Actions == nil {
			actions = defaultActions
		} else {
			actions, err = ParseScanActions(rawScan.Actions)
			if err != nil {
				return nil, err
			}
		}
		scanType, err := ParseTaskType(rawScan.Type)
		if err != nil {
			return nil, err
		}
		diskMode, err := ParseDiskMode(rawScan.DiskMode)
		if err != nil {
			return nil, err
		}
		task, err := NewScanTask(scanType, rawScan.CloudID, scannerHostname, rawScan.Hostname, actions, config.Roles, diskMode)
		if err != nil {
			return nil, err
		}
		if config.Type == ConfigTypeAWS && task.CloudID.Provider != CloudProviderAWS {
			return nil, fmt.Errorf("invalid cloud resource identifier %q: expecting cloud provider %s", rawScan.CloudID, CloudProviderAWS)
		}
		config.Tasks = append(config.Tasks, task)
	}
	return &config, nil
}
