// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package startchecker

import "os"

type ApiKeyEnvRule struct{}

const (
	kmsAPIKeyEnvVar            = "DD_KMS_API_KEY"
	secretsManagerAPIKeyEnvVar = "DD_API_KEY_SECRET_ARN"
	apiKeyEnvVar               = "DD_API_KEY"
)

func (r *ApiKeyEnvRule) ok() bool {
	if len(os.Getenv(kmsAPIKeyEnvVar)) == 0 &&
		len(os.Getenv(secretsManagerAPIKeyEnvVar)) == 0 &&
		len(os.Getenv(apiKeyEnvVar)) == 0 {
		return false
	}
	return true
}

func (r *ApiKeyEnvRule) getError() string {
	return "you need to specify a Datadog API KEY"
}
