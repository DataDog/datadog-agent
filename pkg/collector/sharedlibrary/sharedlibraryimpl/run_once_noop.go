// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !sharedlibrarycheck

package sharedlibrarycheck

import (
	"errors"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

// RunOnce is a no-op when the agent is built without the sharedlibrarycheck tag.
func RunOnce(_ sender.SenderManager, _ string, _ string, _, _ integration.Data) error {
	return errors.New("shared library checks are not enabled (missing sharedlibrarycheck build tag)")
}
