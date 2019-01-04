// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cri

package cri

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() {
	diagnosis.Register("CRI availability", diagnose)
}

// diagnose the CRI socket connectivity
func diagnose() error {
	_, err := GetUtil()
	if err != nil {
		log.Error(err)
	}
	return err
}
