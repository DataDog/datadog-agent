// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package components

import "github.com/DataDog/datadog-agent/test/e2e-framework/components/activedirectory"

// RemoteActiveDirectory represents an Active Directory domain on a remote machine
type RemoteActiveDirectory struct {
	activedirectory.Output
}
