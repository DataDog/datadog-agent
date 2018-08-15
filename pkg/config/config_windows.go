// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

const (
	defaultConfdPath            = filepath.Join(os.Getenv("ProgramData"), "Datadog", "conf.d")
	defaultAdditionalChecksPath = filepath.Join(os.Getenv("ProgramData"), "Datadog", "checks.d")
	defaultRunPath              = ""
	defaultSyslogURI            = ""
	defaultGuiPort              = "5002"
)

// NewAssetFs  Should never be called on non-android
func setAssetFs() {}
