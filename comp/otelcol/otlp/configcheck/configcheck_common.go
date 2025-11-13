// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package configcheck exposes helpers to fetch config.
package configcheck

import (
	"strings"

	"github.com/mohae/deepcopy"

	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// confmapKeyDelimiter is the delimiter used for keys in go.opentelemetry.io/collector/confmap
// We hardcode it to avoid the import to the dependency
const confmapKeyDelimiter = "::"
const viperKeyDelimiter = "."

func readConfigSection(cfg configmodel.Reader, section string) map[string]interface{} {
	//Well, I suppose we have some other use-case in our usage scenario, the function UnmarshalKey allows to read one-by-one config values by key+delimiter to some local variable like int, string, etc.
	//And then use this local variables to apply some settings.
	//As well, some test scenarios with nil sections and use of UnmarshalKey seems to fail.
	//My expectations was that env. variables values will be resolved automatically during config yaml file parsing & reading and merged with config object, so there will be no need to do some extra manual steps.
	//Currently we already have manual resolution of env. variables depending on their type (int or string) but merging of env variables and final config doesn't work well with nested configs.
	//Ok, I will add manual processing of every env. variable and add local patch for this special use case to satisfy defaults requirements by @hush-hush.

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

	// we check every key manually, and if it belongs to the OTLP receiver section,
	// we set it. We need to do this to account for environment variable values.
	prefix := section + viperKeyDelimiter
	for _, key := range cfg.AllKeysLowercased() {
		//The function cfg.IsSet() is deprecated, but recommended function cfg.IsConfigured() is returing false
		//for empty configuration like that:
		// > otlp_config:
		// >   logs:
		// >     enabled:
		// even if default values are set:
		// config.BindEnvAndSetDefault("otlp_config.logs.enabled", false)
		// config.BindEnvAndSetDefault("otlp_config.logs.batch.min_size", 8192)
		//...
		if strings.HasPrefix(key, prefix) && cfg.IsSet(key) {
			var val interface{}
			if _, ok := intConfigs[key]; ok {
				val = deepcopy.Copy(cfg.GetInt(key)) // ensure to get an int even if it is set as a string in env vars
			} else {
				val = deepcopy.Copy(cfg.Get(key))
			}

			//Current function is in charge of reading config section, associated env. variables, def values and then rely on:
			//https://github.com/knadh/koanf/blob/8818413a8bea058821f7bedd8fa208c2b30f8a7a/maps/maps.go#L71
			//to merge all into single configuration tree.
			//Let's take an example of configuration options from
			//https://github.com/DataDog/datadog-agent/blob/9675382ab1887d4003532e7626b176fbb03f509a/pkg/config/config_template.yaml#L4537
			//User specify endpoint in yaml file as 0.0.0.0:4318:
			// a)  config file:
			//     otlp_config:
			//       receiver:
			//         protocols:
			//           http:
			//             endpoint: 0.0.0.0:4318
			// b) But later, user apply also env. variable (it has higher priority comparing to config file):
			//   DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_HTTP_ENDPOINT 0.0.0.0:1234
			// ***************************************************************************************************************
			// Current function will generate the following tree(M):
			//   protocols:
			//     http:
			//       endpoint: 0.0.0.0:4318
			//   protocols::http::endpoint: 0.0.0.0:1234
			//And then the merging function https://github.com/knadh/koanf/blob/8818413a8bea058821f7bedd8fa208c2b30f8a7a/maps/maps.go#L71
			//will try to merge value protocols::http::endpoint into the tree using function Unflatten.
			//The problem is that Unflatten is using iterator over the map M, and due to the nature of golang hash map - iterator won't
			//iterate every time using the same order: sometimes `protocols::http::endpoint` will be applied on top of existing tree and sometimes tree will be applied
			//on top of `protocols::http::endpoint` generating wrong result.
			//****************************************************************************************************************
			//After discussion with configuration team members they:
			//  - confirm that master Viper branch and DD Viper fork are too far to use cherry picks
			//  - issue https://github.com/spf13/viper/issues/1012 is fixed locally in DD fork at:
			//    https://github.com/DataDog/viper/blob/b33ffa9792d9fcd26c68d16c55f17c98cc56c843/viper.go#L908
			//  - Confirm that we have to process env. variables using UnmarshalKey
			//  - Confirm that env variable resolving has to be done manually one by one using UnmarshalKey.
			//Code below is in charge of checking that env. variable is already part of the tree
			//  a) if yes - replace tree value by key
			//  b) if not - keep flatten key-value entry (like `protocols::http::endpoint: 0.0.0.0:1234`) and rely on https://github.com/knadh/koanf/blob/8818413a8bea058821f7bedd8fa208c2b30f8a7a/maps/maps.go#L71
			//    to merge them.

			//preparing key using expected delimeter and removing prefix
			mapKey := strings.ReplaceAll(key[len(prefix):], viperKeyDelimiter, confmapKeyDelimiter)
			keys := strings.Split(mapKey, confmapKeyDelimiter)
			next := stringMap
			keyExists := true

			//seraching over all sub-keys
			for _, k := range keys[:len(keys)-1] {
				sub, ok := next[k]
				if !ok {
					keyExists = false
					break
				}
				if n, ok := sub.(map[string]interface{}); ok {
					next = n
				} else {
					keyExists = false
					break
				}
			}

			//if key __already__ exists in configuration map => replace the value by env. variable value because of higher priority
			if keyExists {
				next[keys[len(keys)-1]] = val
			} else {
				//if not => keep flatten key-value and rely on Unflatten function to do the merge.
				stringMap[mapKey] = val
			}
		}
	}
	return stringMap
}

// intConfigs has the known config keys that may need a type cast to int by calling cfg.GetInt
var intConfigs = map[string]struct{}{
	"otlp_config.receiver.protocols.grpc.max_concurrent_streams": {},
	"otlp_config.receiver.protocols.grpc.max_recv_msg_size_mib":  {},
	"otlp_config.receiver.protocols.grpc.read_buffer_size":       {},
	"otlp_config.receiver.protocols.grpc.write_buffer_size":      {},
	"otlp_config.receiver.protocols.http.max_request_body_size":  {},
	"otlp_config.logs.batch.min_size":                            {},
	"otlp_config.logs.batch.max_size":                            {},
	"otlp_config.metrics.batch.min_size":                         {},
	"otlp_config.metrics.batch.max_size":                         {},
}

// hasSection checks if a subsection of otlp_config section exists in a given config
// It does not handle sections nested further down.
func hasSection(cfg configmodel.Reader, section string) bool {
	// HACK: We want to mark as enabled if the section is present, even if empty, so that we get errors
	// from unmarshaling/validation done by the Collector code.
	//
	// IsSet won't work here: it will return false if the section is present but empty.
	// To work around this, we check if the receiver key is present in the string map, which does the 'correct' thing.
	configSection := readConfigSection(cfg, coreconfig.OTLPSection)
	sectionWithDelimiter := section + confmapKeyDelimiter
	for key := range configSection {
		if key == section || strings.HasPrefix(key, sectionWithDelimiter) {
			return true
		}
	}
	return false
}

// IsConfigEnabled checks if OTLP pipeline is enabled in a given config.
func IsConfigEnabled(cfg configmodel.Reader) bool {
	return hasSection(cfg, coreconfig.OTLPReceiverSubSectionKey)
}
