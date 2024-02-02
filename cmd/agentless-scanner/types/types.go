// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package types holds the different types and constants used in the agentless
// scanner. Most of these types are serializable.
package types

import (
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
	"github.com/aws/aws-sdk-go-v2/aws/arn"
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
	// AWSScan is the type of the scan configuration for AWS
	AWSScan ConfigType = "aws-scan"
)

// ScanType is the type of the scan
type ScanType string

const (
	// HostScanType is the type of the scan for a host
	HostScanType ScanType = "localhost-scan"
	// EBSScanType is the type of the scan for an EBS volume
	EBSScanType ScanType = "ebs-volume"
	// LambdaScanType is the type of the scan for a Lambda function
	LambdaScanType ScanType = "lambda"
)

// ScanAction is the action to perform during the scan
type ScanAction string

const (
	// Malware is the action to scan for malware
	Malware ScanAction = "malware"
	// VulnsHost is the action to scan for vulnerabilities on hosts
	VulnsHost ScanAction = "vulns"
	// VulnsContainers is the action to scan for vulnerabilities in containers
	VulnsContainers ScanAction = "vulnscontainers"
)

// DiskMode is the mode to attach the disk
type DiskMode string

const (
	// VolumeAttach is the mode to attach the disk as a volume
	VolumeAttach DiskMode = "volume-attach"
	// NBDAttach is the mode to attach the disk as a NBD
	NBDAttach DiskMode = "nbd-attach"
	// NoAttach is the mode to not attach the disk
	NoAttach DiskMode = "no-attach"
)

// ScannerName is the name of the scanner
type ScannerName string

const (
	// ScannerNameHostVulns is the name of the scanner for host vulnerabilities
	ScannerNameHostVulns ScannerName = "hostvulns"
	// ScannerNameHostVulnsEBS is the name of the scanner for EBS host vulnerabilities
	ScannerNameHostVulnsEBS ScannerName = "hostvulns-ebs"
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

// RolesMapping is the mapping of roles from accounts IDs to role ARNs
type RolesMapping map[string]*arn.ARN

// ScanConfigRaw is the raw representation of the scan configuration received
// from RC.
type ScanConfigRaw struct {
	Type  string `json:"type"`
	Tasks []struct {
		Type     string   `json:"type"`
		ARN      string   `json:"arn"`
		Hostname string   `json:"hostname"`
		Actions  []string `json:"actions,omitempty"`
	} `json:"tasks"`
	Roles    []string `json:"roles"`
	DiskMode string   `json:"disk_mode"`
}

// ScanConfig is the representation of the scan configuration after being
// parsed and normalized.
type ScanConfig struct {
	Type     ConfigType
	Tasks    []*ScanTask
	Roles    RolesMapping
	DiskMode DiskMode
}

// ScanTask is the representation of a scan task that performs a scan on a
// resource.
type ScanTask struct {
	ID              string       `json:"ID"`
	CreatedAt       time.Time    `json:"CreatedAt"`
	StartedAt       time.Time    `json:"StartedAt"`
	Type            ScanType     `json:"Type"`
	ARN             arn.ARN      `json:"ARN"`
	TargetHostname  string       `json:"Hostname"`
	ScannerHostname string       `json:"ScannerHostname"`
	Actions         []ScanAction `json:"Actions"`
	Roles           RolesMapping `json:"Roles"`
	DiskMode        DiskMode     `json:"DiskMode"`

	// Lifecycle metadata of the task
	CreatedSnapshots        map[string]*time.Time `json:"CreatedSnapshots"`
	AttachedDeviceName      *string               `json:"AttachedDeviceName"`
	AttachedVolumeARN       *arn.ARN              `json:"AttachedVolumeARN"`
	AttachedVolumeCreatedAt *time.Time            `json:"AttachedVolumeCreatedAt"`
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

	// Vulns specific
	SnapshotARN *arn.ARN `json:"SnapshotARN"` // TODO: deprecate as we remove "vm" mode
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
	if o.SnapshotARN != nil {
		h.Write([]byte(o.SnapshotARN.String()))
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

// MakeScanTaskID builds a unique task ID.
func MakeScanTaskID(s *ScanTask) string {
	h := sha256.New()
	createdAt, _ := s.CreatedAt.MarshalBinary()
	h.Write(createdAt)
	h.Write([]byte(s.Type))
	h.Write([]byte(s.ARN.String()))
	h.Write([]byte(s.TargetHostname))
	h.Write([]byte(s.ScannerHostname))
	h.Write([]byte(s.DiskMode))
	for _, action := range s.Actions {
		h.Write([]byte(action))
	}
	return string(s.Type) + "-" + hex.EncodeToString(h.Sum(nil)[:8])
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

func (s *ScanTask) String() string {
	if s == nil {
		return "nilscan"
	}
	return s.ID
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
