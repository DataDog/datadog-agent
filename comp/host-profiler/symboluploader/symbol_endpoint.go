// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package symboluploader

type SymbolEndpoint struct {
	Site   string `mapstructure:"site" json:"site"`
	APIKey string `mapstructure:"api_key" json:"api_key"`
}
