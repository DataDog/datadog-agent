// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package admission

import (
	"github.com/DataDog/datadog-agent/pkg/errors"
	"k8s.io/client-go/discovery"
)

// UseAdmissionV1 discovers which admissionregistration version should be used between v1beta1 and v1.
func UseAdmissionV1(client discovery.DiscoveryInterface) (bool, error) {
	groups, err := client.ServerGroups()
	if err != nil {
		return false, err
	}

	admission := "admissionregistration.k8s.io"
	for _, group := range groups.Groups {
		if group.Name == admission {
			for _, version := range group.Versions {
				if version.Version == "v1" {
					return true, nil
				}
			}
			return false, nil
		}
	}

	return false, errors.NewNotFound(admission)
}
