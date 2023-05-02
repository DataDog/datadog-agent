// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package probe

// ApproverStats is used to collect kernel space metrics about approvers. Stats about added approvers are sent from userspace.
type ApproverStats struct {
	EventApprovedByBasename uint64 `yaml:"event_approved_by_basename"`
	EventApprovedByFlag     uint64 `yaml:"event_approved_by_flag"`
}
