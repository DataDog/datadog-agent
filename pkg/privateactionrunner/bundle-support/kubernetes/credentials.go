// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package kubernetes

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

type KubeConfigCredential struct {
	Context string
}

func parseAsKubeConfigCredentials(credential *privateconnection.PrivateCredentials) (*KubeConfigCredential, bool) {
	if credential == nil {
		return nil, false
	}
	tokens := credential.Tokens
	if len(tokens) != 1 {
		return nil, false
	}
	token := tokens[0]
	if token.Name != "context" {
		return nil, false
	}
	return &KubeConfigCredential{
		Context: token.Value,
	}, true
}

func parseAsServiceAccountCredentials(credential *privateconnection.PrivateCredentials) bool {
	if credential == nil {
		return false
	}
	return len(credential.Tokens) == 0
}
