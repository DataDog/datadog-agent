// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package storeimpl

// TODO(task#13): The previous test suite targeted the old
// ReportIssue(checkID, checkName, *proto.IssueReport) API.
// It has been removed and will be replaced with a proper per-component
// test suite covering:
//   - ReportIssue happy path (IssueId keyed storage, proto fields filled)
//   - ReportIssue validation (nil, empty IssueId, empty IssueType, unknown type)
//   - State machine (new → ongoing on re-report, resolved on ResolveIssue)
//   - Multi-instance: one source can hold many concurrent issue ids
//   - Persistence v2 round-trip and version-mismatch ignore
//   - HTTP endpoint and flare provider
//   - Telemetry counter labelled by issue_id
