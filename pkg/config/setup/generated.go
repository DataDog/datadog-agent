// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

//
// The following code is generated from the schema and should never be manually edited
//

type delegatedAuthConfig struct {
	apiKeyPath        string
	delegatedAuthPath string
	description       string
}

// delegatedAuthKeys list all the "delegated_auth" configuration section.
// This list is used to fully initialize authentication through cloud provider instead of API key
var delegatedAuthKeys = []delegatedAuthConfig{

	{
		apiKeyPath:        "remote_configuration.api_key",
		delegatedAuthPath: "remote_configuration.delegated_auth",
		description:       "remote_configuration",
	},

	{
		apiKeyPath:        "logs_config.api_key",
		delegatedAuthPath: "logs_config.delegated_auth",
		description:       "logs_config",
	},

	{
		apiKeyPath:        "api_key",
		delegatedAuthPath: "delegated_auth",
		description:       "global",
	},

	{
		apiKeyPath:        "evp_proxy_config.api_key",
		delegatedAuthPath: "evp_proxy_config.delegated_auth",
		description:       "evp_proxy_config",
	},

	{
		apiKeyPath:        "ol_proxy_config.api_key",
		delegatedAuthPath: "ol_proxy_config.delegated_auth",
		description:       "ol_proxy_config",
	},
}
