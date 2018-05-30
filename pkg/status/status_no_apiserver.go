// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !kubeapiserver

package status

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getLeaderElectionDetails() map[string]string {
	log.Info("Not implemented")
	return nil
}
