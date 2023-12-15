// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flags holds flags related files
package flags

const (
	// Global to security-agent

	CfgPath        = "cfgpath"         // CfgPath start subcommand
	SysProbeConfig = "sysprobe-config" // SysProbeConfig start subcommand
	NoColor        = "no-color"        // NoColor start subcommand

	// Start Subcommand

	PidFile = "pidfile" // PidFile start subcommand

	// Status Subcommand

	JSON       = "json"        // JSON status subcommand
	PrettyJSON = "pretty-json" // PrettyJSON status subcommand
	File       = "file"        // File status subcommand

	// Flare Subcommand

	Email = "email" // Email flare subcommand
	Send  = "send"  // Send flare subcommand

	// Runtime Subcommand

	PoliciesDir        = "policies-dir"        // PoliciesDir runtime subcommand
	EventFile          = "event-file"          // EventFile runtime subcommand
	Debug              = "debug"               // Debug runtime subcommand
	Check              = "check"               // Check runtime subcommand
	OutputPath         = "output-path"         // OutputPath runtime subcommand
	WithArgs           = "with-args"           // WithArgs runtime subcommand
	SnapshotInterfaces = "snapshot-interfaces" // SnapshotInterfaces runtime subcommand
	RuleID             = "rule-id"             // RuleID Also for compliance subcommand

	// Runtime Policy Check Subcommand

	EvaluateLoadedPolicies = "loaded-policies" // EvaluateLoadedPolicies policy check subcommand

	// Runtime Activity Dump Subcommand

	Name              = "name"               // Name activity dump subcommand
	ContainerID       = "containerID"        // ContainerID activity dump subcommand
	Comm              = "comm"               // Comm activity dump subcommand
	Timeout           = "timeout"            // Timeout activity dump subcommand
	DifferentiateArgs = "differentiate-args" // DifferentiateArgs activity dump subcommand
	Output            = "output"             // Output TODO: unify with OutputPath
	Compression       = "compression"        // Compression activity dump subcommand
	Format            = "format"             // Format activity dump subcommand
	RemoteCompression = "remote-compression" // RemoteCompression activity dump subcommand
	RemoteFormat      = "remote-format"      // RemoteFormat activity dump subcommand
	Input             = "input"              // Input activity dump subcommand
	Remote            = "remote"             // Remote activity dump subcommand
	Origin            = "origin"             // Origin activity dump subcommand
	Target            = "target"             // Target activity dump subcommand

	// Security Profile Subcommand

	SecurityProfileInput = "input"         // SecurityProfileInput security profile subcommand
	IncludeCache         = "include-cache" // IncludeCache security profile subcommand
	ImageName            = "name"          // ImageName security profile subcommand
	ImageTag             = "tag"           // ImageTag security profile subcommand

	// Compliance Subcommand

	SourceType   = "source-type"   // SourceType compliance subcommand
	SourceName   = "source-name"   // SourceName compliance subcommand
	ResourceID   = "resource-id"   // ResourceID compliance subcommand
	ResourceType = "resource-type" // ResourceType compliance subcommand
	Tags         = "tags"          // Tags compliance subcommand
	Data         = "data"          // Data compliance subcommand

	// Check Subcommand

	Framework         = "framework"           // Framework check subcommand
	Verbose           = "verbose"             // Verbose check subcommand
	Report            = "report"              // Report check subcommand
	OverrideRegoInput = "override-rego-input" // OverrideRegoInput check subcommand
	DumpReports       = "dump-reports"        // DumpReports TODO: Unify with OutputPath
)
