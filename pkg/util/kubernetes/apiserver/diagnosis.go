// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() {
	diagnosis.Register("Kubernetes API Server availability", diagnose)
}

// diagnose the API server availability
func diagnose() error {
	isConnectVerbose = true
	c, err := GetAPIClient()
	isConnectVerbose = false
	if err != nil {
		log.Error(err)
		return err
	}
	log.Infof("Detecting OpenShift APIs: %s available", c.DetectOpenShiftAPILevel())
	return nil
}
