// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !aix

package flare

import (
	"context"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
)

// getSvmonData is a no-op outside of AIX, which is the only platform with the
// svmon command.
func getSvmonData(_ context.Context, _ flaretypes.FlareBuilder) error {
	return nil
}
