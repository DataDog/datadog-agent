// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"errors"
	"fmt"
)

// ResourceKind represents resource kind
type ResourceKind string

const (
	// KindInvalid is set in case resource is invalid
	KindInvalid = ResourceKind("invalid")
	// KindFile is used for a file resource
	KindFile = ResourceKind("file")
	// KindProcess is used for a Process resource
	KindProcess = ResourceKind("process")
	// KindGroup is used for a Group resource
	KindGroup = ResourceKind("group")
	// KindCommand is used for a Command resource
	KindCommand = ResourceKind("command")
	// KindDocker is used for a DockerResource resource
	KindDocker = ResourceKind("docker")
	// KindAudit is used for an Audit resource
	KindAudit = ResourceKind("audit")
	// KindKubernetes is used for a KubernetesResource
	KindKubernetes = ResourceKind("kubernetes")
	// KindCustom is used for a Custom check
	KindCustom = ResourceKind("custom")
)

// Resource describes supported resource types observed by a Rule
type Resource struct {
	File          *File               `yaml:"file,omitempty"`
	Process       *Process            `yaml:"process,omitempty"`
	Group         *Group              `yaml:"group,omitempty"`
	Command       *Command            `yaml:"command,omitempty"`
	Audit         *Audit              `yaml:"audit,omitempty"`
	Docker        *DockerResource     `yaml:"docker,omitempty"`
	KubeApiserver *KubernetesResource `yaml:"kubeApiserver,omitempty"`
	Custom        *Custom             `yaml:"custom,omitempty"`
	Condition     string              `yaml:"condition"`
	Fallback      *Fallback           `yaml:"fallback,omitempty"`
}

// RegoResource describes supported resource types observed by a Rego Rule
type RegoResource struct {
	File          *File               `yaml:"file,omitempty"`
	Process       *Process            `yaml:"process,omitempty"`
	Group         *Group              `yaml:"group,omitempty"`
	Command       *Command            `yaml:"command,omitempty"`
	Audit         *Audit              `yaml:"audit,omitempty"`
	Docker        *DockerResource     `yaml:"docker,omitempty"`
	KubeApiserver *KubernetesResource `yaml:"kubeApiserver,omitempty"`
	Custom        *Custom             `yaml:"custom,omitempty"`
}

// Kind returns ResourceKind of the resource
func (r *Resource) Kind() ResourceKind {
	switch {
	case r.File != nil:
		return KindFile
	case r.Process != nil:
		return KindProcess
	case r.Group != nil:
		return KindGroup
	case r.Command != nil:
		return KindCommand
	case r.Audit != nil:
		return KindAudit
	case r.Docker != nil:
		return KindDocker
	case r.KubeApiserver != nil:
		return KindKubernetes
	case r.Custom != nil:
		return KindCustom
	default:
		return KindInvalid
	}
}

// Fallback specifies optional fallback configuration for a resource
type Fallback struct {
	Condition string   `yaml:"condition,omitempty"`
	Resource  Resource `yaml:"resource"`
}

// Fields & functions available for File
const (
	FileFieldPath        = "file.path"
	FileFieldPermissions = "file.permissions"
	FileFieldUser        = "file.user"
	FileFieldGroup       = "file.group"

	FileFuncJQ     = "file.jq"
	FileFuncYAML   = "file.yaml"
	FileFuncRegexp = "file.regexp"
)

// File describes a file resource
type File struct {
	Path string `yaml:"path"`
}

// Fields & functions available for Process
const (
	ProcessFieldName    = "process.name"
	ProcessFieldExe     = "process.exe"
	ProcessFieldCmdLine = "process.cmdLine"

	ProcessFuncFlag    = "process.flag"
	ProcessFuncHasFlag = "process.hasFlag"
)

// Process describes a process resource
type Process struct {
	Name string `yaml:"name"`
}

// Fields & functions available for KubernetesResource
const (
	KubeResourceFieldName      = "kube.resource.name"
	KubeResourceFieldGroup     = "kube.resource.group"
	KubeResourceFieldVersion   = "kube.resource.version"
	KubeResourceFieldNamespace = "kube.resource.namespace"
	KubeResourceFieldKind      = "kube.resource.kind"

	KubeResourceFuncJQ = "kube.resource.jq"
)

// KubernetesResource describes any object in Kubernetes (incl. CRDs)
type KubernetesResource struct {
	Kind      string `yaml:"kind"`
	Version   string `yaml:"version,omitempty"`
	Group     string `yaml:"group,omitempty"`
	Namespace string `yaml:"namespace,omitempty"`

	// A selector to restrict the list of returned objects by their labels.
	// Defaults to everything.
	LabelSelector string `yaml:"labelSelector,omitempty"`
	// A selector to restrict the list of returned objects by their fields.
	// Defaults to everything.
	FieldSelector string `yaml:"fieldSelector,omitempty"`

	APIRequest KubernetesAPIRequest `yaml:"apiRequest"`
}

// String returns human-friendly information string about the KubernetesResource
func (kr *KubernetesResource) String() string {
	return fmt.Sprintf("%s/%s - Kind: %s - Namespace: %s - Request: %s - %s", kr.Group, kr.Version, kr.Kind, kr.Namespace, kr.APIRequest.Verb, kr.APIRequest.ResourceName)
}

// KubernetesAPIRequest defines it check applies to a single object or a list
type KubernetesAPIRequest struct {
	Verb         string `yaml:"verb"`
	ResourceName string `yaml:"resourceName,omitempty"`
}

// Fields & functions available for Group
const (
	GroupFieldName  = "group.name"
	GroupFieldUsers = "group.users"
	GroupFieldID    = "group.id"
)

// Group describes a group membership resource
type Group struct {
	Name string `yaml:"name"`
}

// BinaryCmd describes a command in form of a name + args
type BinaryCmd struct {
	Name string   `yaml:"name"`
	Args []string `yaml:"args,omitempty"`
}

func (c *BinaryCmd) String() string {
	return fmt.Sprintf("Binary command: %s, args: %v", c.Name, c.Args)
}

// ShellCmd describes a command to be run through a shell
type ShellCmd struct {
	Run   string     `yaml:"run"`
	Shell *BinaryCmd `yaml:"shell,omitempty"`
}

func (c *ShellCmd) String() string {
	return fmt.Sprintf("Shell command: %s", c.Run)
}

// Fields & functions available for Command
const (
	CommandFieldExitCode = "command.exitCode"
	CommandFieldStdout   = "command.stdout"
)

// Command describes a command resource usually reporting exit code or output
type Command struct {
	BinaryCmd      *BinaryCmd `yaml:"binary,omitempty"`
	ShellCmd       *ShellCmd  `yaml:"shell,omitempty"`
	TimeoutSeconds int        `yaml:"timeout,omitempty"`
}

func (c *Command) String() string {
	if c.BinaryCmd != nil {
		return c.BinaryCmd.String()
	}
	if c.ShellCmd != nil {
		return c.ShellCmd.String()
	}
	return "Empty command"
}

// Fields & functions available for Audit
const (
	AuditFieldPath        = "audit.path"
	AuditFieldEnabled     = "audit.enabled"
	AuditFieldPermissions = "audit.permissions"
)

// Audit describes an audited file resource
type Audit struct {
	Path string `yaml:"path"`
}

// Validate validates audit resource
func (a *Audit) Validate() error {
	if len(a.Path) == 0 {
		return errors.New("audit resource is missing path")
	}
	return nil
}

// Fields & functions available for Docker
const (
	DockerImageFieldID   = "image.id"
	DockerImageFieldTags = "image.tags"

	DockerContainerFieldID    = "container.id"
	DockerContainerFieldName  = "container.name"
	DockerContainerFieldImage = "container.image"

	DockerNetworkFieldID   = "network.id"
	DockerNetworkFieldName = "network.name"

	DockerVersionFieldVersion       = "docker.version"
	DockerVersionFieldAPIVersion    = "docker.apiVersion"
	DockerVersionFieldPlatform      = "docker.platform"
	DockerVersionFieldExperimental  = "docker.experimental"
	DockerVersionFieldOS            = "docker.os"
	DockerVersionFieldArch          = "docker.arch"
	DokcerVersionFieldKernelVersion = "docker.kernelVersion"

	DockerFuncTemplate = "docker.template"
)

// DockerResource describes a resource from docker daemon
type DockerResource struct {
	Kind string `yaml:"kind"`
}

// Custom is a special resource handled by a dedicated function
type Custom struct {
	Name      string            `yaml:"name"`
	Variables map[string]string `yaml:"variables,omitempty"`
}
