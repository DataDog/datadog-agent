// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package filepermissions

// FilePermissions interface defines all commands that can be used to set and reset file permissions.
// These functions are used to create args.Command that will be run by pulumi.
type FilePermissions interface {
	SetupPermissionsCommand(path string) string
	ResetPermissionsCommand(path string) string
}
