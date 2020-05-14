// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package mutate

import admiv1beta1 "k8s.io/api/admission/v1beta1"

func InjectConfig(req *admiv1beta1.AdmissionRequest) (*admiv1beta1.AdmissionResponse, error) {
	// TODO: implement me
	return nil, nil
}

func InjectTags(req *admiv1beta1.AdmissionRequest) (*admiv1beta1.AdmissionResponse, error) {
	// TODO: implement me
	return nil, nil
}
