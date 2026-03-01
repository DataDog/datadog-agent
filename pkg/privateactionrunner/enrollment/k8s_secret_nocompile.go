// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !kubeapiserver

package enrollment

import (
	"context"
	"errors"

	configModel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func getIdentityFromK8sSecret(_ context.Context, _ configModel.Reader) (*PersistedIdentity, error) {
	return nil, errors.New("Kubernetes secret storage is not available in this build")
}

func persistIdentityToK8sSecret(_ context.Context, _ configModel.Reader, _ *Result) error {
	return errors.New("Kubernetes secret storage is not available in this build")
}
