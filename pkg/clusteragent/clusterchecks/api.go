// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import "github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"

// GetAllConfigs returns all configurations known to the store, for reporting
func (h *Handler) GetAllConfigs() ([]integration.Config, error) {
	return h.store.getAllConfigs(), nil
}
