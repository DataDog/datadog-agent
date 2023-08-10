// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flags TODO comment
package flags

// This const block should have a comment or be unexported
const (
	// CfgPath defines the cfgpath flag
	// Start Subcommand
	CfgPath = "cfgpath"
	NoColor = "no-color"
	PidFile = "pidfile"

	// JSON Status Subcommand
	JSON       = "json"
	PrettyJSON = "pretty-json"
	File       = "file" // File Also for check subcommand

	// Email Flare Subcommand
	Email = "email"
	// Send exported const should have comment (or a comment on this block) or be unexported
	Send = "send"

	// PoliciesDir Runtime Subcommand
	PoliciesDir        = "policies-dir"
	EventFile          = "event-file"
	Debug              = "debug"
	Check              = "check"
	OutputPath         = "output-path"
	WithArgs           = "with-args"
	SnapshotInterfaces = "snapshot-interfaces"
	RuleID             = "rule-id" // RuleID Also for compliance subcommand

	// EvaluateLoadedPolicies Runtime Policy Check Subcommand
	EvaluateLoadedPolicies = "loaded-policies"

	// Name Runtime Activity Dump Subcommand
	Name              = "name"
	ContainerID       = "containerID"
	Comm              = "comm"
	Timeout           = "timeout"
	DifferentiateArgs = "differentiate-args"
	Output            = "output" // Output TODO: unify with OutputPath
	Compression       = "compression"
	Format            = "format"
	RemoteCompression = "remote-compression"
	RemoteFormat      = "remote-format"
	Input             = "input"
	Remote            = "remote"
	Origin            = "origin"
	Target            = "target"

	// SecurityProfileInput Security Profile Subcommand
	SecurityProfileInput = "input"
	IncludeCache         = "include-cache"
	ImageName            = "name"
	ImageTag             = "tag"

	// SourceType Compliance Subcommand
	SourceType   = "source-type"
	SourceName   = "source-name"
	ResourceID   = "resource-id"
	ResourceType = "resource-type"
	Tags         = "tags"
	Data         = "data"

	// Framework Check Subcommand
	Framework         = "framework"
	Verbose           = "verbose"
	Report            = "report"
	OverrideRegoInput = "override-rego-input"
	DumpReports       = "dump-reports" // DumpReports TODO: Unify with OutputPath
)
