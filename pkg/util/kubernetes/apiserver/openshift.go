// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DetectOpenShiftAPILevel looks at known endpoints to detect if OpenShift
// APIs are available on this apiserver. OpenShift transitioned from a
// non-standard `/oapi` URL prefix to standard api groups under the `/apis`
// prefix in 3.6. Detecting both, with a preference for the new prefix.
func (c *APIClient) DetectOpenShiftAPILevel() OpenShiftAPILevel {
	err := c.Cl.CoreV1().RESTClient().Get().AbsPath("/apis/quota.openshift.io").Do(context.TODO()).Error()
	if err == nil {
		log.Debugf("Found %s", OpenShiftAPIGroup)
		return OpenShiftAPIGroup
	}
	log.Debugf("Cannot access %s: %s", OpenShiftAPIGroup, err)

	err = c.Cl.CoreV1().RESTClient().Get().AbsPath("/oapi").Do(context.TODO()).Error()
	if err == nil {
		log.Debugf("Found %s", OpenShiftOAPI)
		return OpenShiftOAPI
	}
	log.Debugf("Cannot access %s: %s", OpenShiftOAPI, err)

	// Fallback to NotOpenShift
	return NotOpenShift
}
