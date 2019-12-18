// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver,secrets

package app

import (
	"github.com/DataDog/datadog-agent/pkg/secrets"
)

func init() {
	ClusterAgentCmd.AddCommand(secrets.ReadSecretCmd)
}
