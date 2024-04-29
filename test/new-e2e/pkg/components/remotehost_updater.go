// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package components

import (
	"github.com/DataDog/test-infra-definitions/components/datadog/updater"
)

// RemoteHostUpdater represents an Updater running directly on a Host
type RemoteHostUpdater struct {
	updater.HostUpdaterOutput

	// add Client when a test needs to interact with the updater
}
