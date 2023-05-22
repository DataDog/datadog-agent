// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flags

const (
	// Start Subcommand
	// CfgPath defines the cfgpath flag
	CfgPath = "cfgpath"
	NoColor = "no-color"
	PidFile = "pidfile"

	// Status Subcommand
	JSON       = "json"
	PrettyJSON = "pretty-json"
	File       = "file" // Also for check subcommand

	// Flare Subcommand
	Email = "email"
	Send  = "send"

	// Runtime Subcommand
	PoliciesDir        = "policies-dir"
	EventFile          = "event-file"
	Debug              = "debug"
	Check              = "check"
	OutputPath         = "output-path"
	WithArgs           = "with-args"
	SnapshotInterfaces = "snapshot-interfaces"
	RuleID             = "rule-id" // Also for compliance subcommand

	// Runtime Activity Dump Subcommand
	Name              = "name"
	ContainerID       = "containerID"
	Comm              = "comm"
	Timeout           = "timeout"
	DifferentiateArgs = "differentiate-args"
	Output            = "output" // TODO: unify with OutputPath
	Compression       = "compression"
	Format            = "format"
	RemoteCompression = "remote-compression"
	RemoteFormat      = "remote-format"
	Input             = "input"
	Remote            = "remote"

	// Compliance Subcommand
	SourceType   = "source-type"
	SourceName   = "source-name"
	ResourceID   = "resource-id"
	ResourceType = "resource-type"
	Tags         = "tags"
	Data         = "data"

	// Check Subcommand
	Framework         = "framework"
	Verbose           = "verbose"
	Report            = "report"
	OverrideRegoInput = "override-rego-input"
	DumpReports       = "dump-reports" // TODO: Unify with OutputPath
)
