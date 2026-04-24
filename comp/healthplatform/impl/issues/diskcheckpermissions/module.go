// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package diskcheckpermissions provides an issue module for disk check permission errors.
// This module only provides remediation (no built-in check) as disk permission errors
// are reported by external integrations (the disk check collector).
package diskcheckpermissions

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for disk check permission denied issues
	IssueID = "disk-check-permission-denied"
)

// diskCheckPermissionsModule implements issues.Module
type diskCheckPermissionsModule struct {
	template *DiskCheckPermissionIssue
}

// NewModule creates a new disk check permission issue module
func NewModule(config.Component) issues.Module {
	return &diskCheckPermissionsModule{
		template: NewDiskCheckPermissionIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *diskCheckPermissionsModule) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *diskCheckPermissionsModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns nil - disk check permission errors are reported by external integrations
func (m *diskCheckPermissionsModule) BuiltInCheck() *issues.BuiltInCheck {
	return nil
}
