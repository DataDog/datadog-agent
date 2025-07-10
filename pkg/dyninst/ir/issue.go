// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ir

// ProbeIssue is an issue that was encountered while processing a probe.
type ProbeIssue struct {
	ProbeDefinition
	Issue
}

//go:generate go run golang.org/x/tools/cmd/stringer -type=IssueKind -trimprefix=IssueKind

// IssueKind is the kind of issue that was encountered.
type IssueKind int

const (
	_ IssueKind = iota
	// IssueKindInvalidProbeDefinition is an issue that was encountered while
	// deserializing a probe definition.
	IssueKindInvalidProbeDefinition
	// IssueKindTargetNotFoundInBinary is an issue that was encountered while
	// processing a probe definition and failing to find the target in the
	// binary.
	IssueKindTargetNotFoundInBinary
	// IssueKindUnsupportedFeature is an issue that was encountered while
	// processing a probe definition that uses a feature that is not supported.
	IssueKindUnsupportedFeature
	// IssueKindMalformedExecutable is an issue that was encountered while
	// processing a probe definition that uses a malformed executable.
	IssueKindMalformedExecutable
	// IssueKindInvalidDWARF is an issue that was encountered while processing
	// a probe definition that uses an invalid DWARF.
	IssueKindInvalidDWARF
)

// Issue is an issue that was encountered while processing a probe.
type Issue struct {
	Kind    IssueKind
	Message string
}

// IsNone returns true if the issue is empty.
func (i Issue) IsNone() bool {
	return i == Issue{}
}
