// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package darwin

import (
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usergroup"
)

// newNameResolver returns the macOS user/group resolver, which goes through
// os/user and therefore Open Directory.
func newNameResolver() (nameResolver, error) {
	return usergroup.NewResolver()
}
