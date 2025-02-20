// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/retry"

	"k8s.io/apimachinery/pkg/version"
)

const serverVersionCacheKey = "kubeServerVersion"

// kubeServerVersion is a local struct adapted to the agent retry package.
// It allow retrieving the kubernetes server version with a retry.
type kubeServerVersion struct {
	retrier    retry.Retrier
	clientFunc func() (*version.Info, error)
	info       *version.Info
}

func newKubeServerVersion(retryTimeout time.Duration, discoveryFunc func() (*version.Info, error)) (*kubeServerVersion, error) {
	serverVersion := &kubeServerVersion{clientFunc: discoveryFunc}
	return serverVersion, serverVersion.retrier.SetupRetrier(&retry.Config{
		Name:              "kubeServerVersion",
		AttemptMethod:     serverVersion.set,
		Strategy:          retry.Backoff,
		InitialRetryDelay: 1 * time.Second,
		MaxRetryDelay:     retryTimeout,
	})
}

// set is a retriable method to retrieve the kubernetes server version.
func (k *kubeServerVersion) set() error {
	var err error
	k.info, err = k.clientFunc()
	return err
}
