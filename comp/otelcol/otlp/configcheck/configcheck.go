// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp

// Package configcheck exposes helpers to fetch config.
package configcheck

import (
	"strings"

	"github.com/mohae/deepcopy"
	"go.opentelemetry.io/collector/confmap"

	"github.com/DataDog/datadog-agent/comp/core/config"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// ReadConfigSection from a config.Component object.
func ReadConfigSection(cfg config.Reader, section string) *confmap.Conf {
	// Viper doesn't work well when getting subsections, since it
	// ignores environment variables and nil-but-present sections.
	// To work around this, we do the following two steps:

	// Step one works around https://github.com/spf13/viper/issues/819
	// If we only had the stuff below, the nil sections would be ignored.
	// We want to take into account nil-but-present sections.
	//
	// Furthermore, Viper returns an `interface{}` nil in the case where
	// `section` is present but empty: e.g. we want to read
	//	"otlp_config.receiver", but we have
	//
	//         otlp_config:
	//           receiver:
	//
	// `GetStringMap` it will fail to cast `interface{}` nil to
	// `map[string]interface{}` nil; we use `Get` and cast manually.
	rawVal := cfg.Get(section)
	stringMap := map[string]interface{}{}
	if val, ok := rawVal.(map[string]interface{}); ok {
		// deep copy since `cfg.Get` returns a reference
		stringMap = deepcopy.Copy(val).(map[string]interface{})
	}

	// Step two works around https://github.com/spf13/viper/issues/1012
	// we check every key manually, and if it belongs to the OTLP receiver section,
	// we set it. We need to do this to account for environment variable values.
	prefix := section + "."
	for _, key := range cfg.AllKeysLowercased() {
		if strings.HasPrefix(key, prefix) && cfg.IsSet(key) {
			mapKey := strings.ReplaceAll(key[len(prefix):], ".", confmap.KeyDelimiter)
			// deep copy since `cfg.Get` returns a reference
			var val interface{}
			if _, ok := intConfigs[key]; ok {
				val = deepcopy.Copy(cfg.GetInt(key)) // ensure to get an int even if it is set as a string in env vars
			} else {
				val = deepcopy.Copy(cfg.Get(key))
			}
			stringMap[mapKey] = val
		}
	}
	return confmap.NewFromStringMap(stringMap)
}

// intConfigs has the known config keys that may need a type cast to int by calling cfg.GetInt
var intConfigs = map[string]struct{}{
	"otlp_config.receiver.protocols.grpc.max_concurrent_streams": {},
	"otlp_config.receiver.protocols.grpc.max_recv_msg_size_mib":  {},
	"otlp_config.receiver.protocols.grpc.read_buffer_size":       {},
	"otlp_config.receiver.protocols.grpc.write_buffer_size":      {},
	"otlp_config.receiver.protocols.http.max_request_body_size":  {},
}

// IsEnabled checks if OTLP pipeline is enabled in a given config.
func IsEnabled(cfg config.Reader) bool {
	return hasSection(cfg, coreconfig.OTLPReceiverSubSectionKey)
}

// HasLogsSectionEnabled checks if OTLP logs are explicitly enabled in a given config.
func HasLogsSectionEnabled(cfg config.Reader) bool {
	return hasSection(cfg, coreconfig.OTLPLogsEnabled) && cfg.GetBool(coreconfig.OTLPLogsEnabled)
}

func hasSection(cfg config.Reader, section string) bool {
	// HACK: We want to mark as enabled if the section is present, even if empty, so that we get errors
	// from unmarshaling/validation done by the Collector code.
	//
	// IsSet won't work here: it will return false if the section is present but empty.
	// To work around this, we check if the receiver key is present in the string map, which does the 'correct' thing.
	_, ok := ReadConfigSection(cfg, coreconfig.OTLPSection).ToStringMap()[section]
	return ok
}

// IsDisplayed checks if the OTLP section should be rendered in the Agent
func IsDisplayed() bool {
	return true
}
