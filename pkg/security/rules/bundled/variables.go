// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package bundled contains bundled rules
package bundled

const (
	needRefreshSBOMVariableScope = "process"
	needRefreshSBOMVariableName  = "pkg_db_modified"
)

// InternalVariables lists all variables used by internal rules
var InternalVariables = [...]string{
	needRefreshSBOMVariableScope + "." + needRefreshSBOMVariableName,
}
