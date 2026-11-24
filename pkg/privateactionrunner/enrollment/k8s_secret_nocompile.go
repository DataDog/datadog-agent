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

var errK8sSecretUnavailable = errors.New("Kubernetes secret storage is not available in this build")

type k8sIdentityStorage struct{}

func newK8sIdentityStorage(_ configModel.Reader, _ LeaderSource) *k8sIdentityStorage {
	return &k8sIdentityStorage{}
}

func (*k8sIdentityStorage) Load(_ context.Context) (*PersistedIdentity, error) {
	return nil, errK8sSecretUnavailable
}

func (*k8sIdentityStorage) Persist(_ context.Context, _ *Result) error {
	return errK8sSecretUnavailable
}

func persistIdentityToK8sSecretNoLeader(_ context.Context, _ configModel.Reader, _ *Result) error {
	return errK8sSecretUnavailable
}
