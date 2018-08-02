// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !kubeapiserver

package leaderelection

import "github.com/DataDog/datadog-agent/pkg/util/log"

// GetStatus returns status info for leader election.
func GetStatus() map[string]interface{} {
	log.Info("Not implemented")
	return nil
}
