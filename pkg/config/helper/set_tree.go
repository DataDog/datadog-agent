// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helper

import (
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func SetTree(cfg model.ReaderWriter, key string, value interface{}, source model.Source) {
	valueMap, ok := value.(map[string]interface{})
	if !ok {
		// not a map, assign to the leaf setting
		cfg.Set(key, value, source)
		return
	}
	if cfg.IsSetting(key) {
		// the value is a map, but the config says this key is a setting
		cfg.Set(key, value, source)
		return
	}
	// otherwise recursively assign subfield settings
	for k, v := range valueMap {
		subkey := key + "." + k
		SetTree(cfg, subkey, v, source)
	}
}
