// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !kubelet

package autodiscovery

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetAutodiscoveryErrors logs the lack of support for getting AD errors on config providers other than the kubelet config provider
func (ac *AutoConfig) GetAutodiscoveryErrors(cpName string) map[string]map[string]bool {
	log.Errorf("Getting autodiscovery errors is unsupported for config provider %v", cpName)
	return nil
}
