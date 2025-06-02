// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

// StringifyConfig defines configuration options for the Stringify method
type StringifyConfig struct {
	DedupPointerAddr bool
	OmitPointerAddr  bool
	SettingFilters   []string
}

// StringifyOption sets an option on the StringifyConfig
type StringifyOption func(*StringifyConfig)

// DedupPointerAddr deduplicates pointers in the Stringify output
func DedupPointerAddr(strcfg *StringifyConfig) {
	strcfg.DedupPointerAddr = true
}

// OmitPointerAddr omits pointer's raw addresses in the Stringify output, useful for testing
func OmitPointerAddr(strcfg *StringifyConfig) {
	strcfg.OmitPointerAddr = true
}

// FilterSettings will filter the output of Stringify to only show the given settings
func FilterSettings(filters []string) StringifyOption {
	return func(strcfg *StringifyConfig) {
		strcfg.SettingFilters = filters
	}
}
