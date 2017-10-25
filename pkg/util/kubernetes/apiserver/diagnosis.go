// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package apiserver

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	log "github.com/cihub/seelog"
	"github.com/ericchiang/k8s"
)

func init() {
	diagnosis.Register("K8s API Server availability", diagnose)
}

// diagnosee the API server availability
func diagnose() error {
	_, err := k8s.NewInClusterClient()
	if err != nil {
		log.Error(err)
	}
	return err
}
