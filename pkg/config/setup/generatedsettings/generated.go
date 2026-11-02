// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// NOTE! This is a generated file, do not modify it. Created by `dda inv schema.codegen`

package generatedsettings

//
// The following code is generated from the schema and should never be manually edited
//

// DelegatedAuthConfig describes a "delegated_auth" configuration section.
type DelegatedAuthConfig struct {
	APIKeyPath        string
	DelegatedAuthPath string
	Description       string
}

// DelegatedAuthKeys list all the "delegated_auth" configuration section.
// This list is used to fully initialize authentication through cloud provider instead of API key
var DelegatedAuthKeys = []DelegatedAuthConfig{
	{
		APIKeyPath:        "remote_configuration.api_key",
		DelegatedAuthPath: "remote_configuration.delegated_auth",
		Description:       "remote_configuration",
	},
	{
		APIKeyPath:        "logs_config.api_key",
		DelegatedAuthPath: "logs_config.delegated_auth",
		Description:       "logs_config",
	},
	{
		APIKeyPath:        "api_key",
		DelegatedAuthPath: "delegated_auth",
		Description:       "global",
	},
	{
		APIKeyPath:        "evp_proxy_config.api_key",
		DelegatedAuthPath: "evp_proxy_config.delegated_auth",
		Description:       "evp_proxy_config",
	},
	{
		APIKeyPath:        "ol_proxy_config.api_key",
		DelegatedAuthPath: "ol_proxy_config.delegated_auth",
		Description:       "ol_proxy_config",
	},
}
