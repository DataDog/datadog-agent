// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"errors"

	osq "github.com/openshift/api/quota/v1"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var ErrNotOpenShift = errors.New("not an OpenShift cluster")

// IsOpenShift detects available endpoints to determine of OpenShift APIs are available
func (c *APIClient) IsOpenShift() OpenShiftApiLevel {
	if c.isOpenShift != OpenShiftUnknown {
		return c.isOpenShift
	}
	err := c.Cl.CoreV1().RESTClient().Get().AbsPath("/apis/quota.openshift.io").Do().Error()
	if err == nil {
		c.isOpenShift = OpenShiftAPIGroup
		return c.isOpenShift
	}
	log.Debugf("Cannot access new OpenShift API: %s", err)

	err = c.Cl.CoreV1().RESTClient().Get().AbsPath("/oapi").Do().Error()
	if err == nil {
		c.isOpenShift = OpenShiftOApi
		return c.isOpenShift
	}
	log.Debugf("Cannot access old OpenShift OAPI: %s", err)

	// Fallback to NotOpenShift
	c.isOpenShift = NotOpenShift
	return c.isOpenShift
}

// ListOShiftClusterQuotas retrieves Openshift ClusterResourceQuota objects
// from the APIserver, returns ErrNotOpenShift if called on a non-Openshift cluster
func (c *APIClient) ListOShiftClusterQuotas() ([]osq.ClusterResourceQuota, error) {
	var url string
	switch c.IsOpenShift() {
	case NotOpenShift:
		return nil, ErrNotOpenShift
	case OpenShiftAPIGroup:
		url = "/apis/quota.openshift.io/v1/clusterresourcequotas/"
		break
	case OpenShiftOApi:
		url = "/oapi/v1/clusterresourcequotas/"
		break
	}

	list := &osq.ClusterResourceQuotaList{}
	err := c.GetRESTObject(url, list)
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}
