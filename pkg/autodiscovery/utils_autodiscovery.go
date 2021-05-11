// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubelet

package autodiscovery

import "github.com/DataDog/datadog-agent/pkg/autodiscovery/providers"

// GetAutodiscoveryErrors fetches AD errors from each ConfigProvider
func (ac *AutoConfig) GetAutodiscoveryErrors() map[string]map[string]providers.ErrorMsgSet {
	errors := map[string]map[string]providers.ErrorMsgSet{}
	for _, pd := range ac.providers {
		errors[pd.provider.String()] = pd.provider.GetConfigErrors()
	}
	return errors
}
