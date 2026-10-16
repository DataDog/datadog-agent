// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"

	"golang.org/x/mod/semver"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
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

// KubeServerVersion returns the version of the kubernetes server using a Discovery Client.
// It retries with an exponential backoff until timeout.
// It caches the result in memory for 1 hour.
func KubeServerVersion(discoveryCl discovery.DiscoveryInterface, retryTimeout time.Duration) (*version.Info, error) {
	if serverVersion, found := cache.Cache.Get(serverVersionCacheKey); found {
		return serverVersion.(*version.Info), nil
	}

	serverVersion, err := newKubeServerVersion(retryTimeout, discoveryCl.ServerVersion)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), retryTimeout)
	defer cancel()

	for {
		err := serverVersion.retrier.TriggerRetry()
		switch serverVersion.retrier.RetryStatus() {
		case retry.OK:
			cache.Cache.Set(serverVersionCacheKey, serverVersion.info, time.Hour)
			return serverVersion.info, nil
		case retry.PermaFail:
			return nil, err
		default:
			sleepFor := serverVersion.retrier.NextRetry().UTC().Sub(time.Now().UTC()) + time.Second
			log.Debugf("Waiting for getting Kubernetes server version, next retry: %v", sleepFor)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("timeout reached while waiting for Kubernetes server version, last error: %w", err)
			case <-time.After(sleepFor):
			}
		}
	}
}

// NormalizeKubeSemver returns the server version as a valid semver string.
// It strips build metadata like "+k3s1" and falls back to Major/Minor
// (e.g. "1" and "22+") when GitVersion is not valid semver.
func NormalizeKubeSemver(v *version.Info) (string, bool) {
	if v == nil {
		return "", false
	}

	// Prefer GitVersion (returned by v.String()) and strip build metadata like "+k3s1".
	s := v.String()
	if idx := strings.IndexByte(s, '+'); idx >= 0 {
		s = s[:idx]
	}
	if semver.IsValid(s) {
		return s, true
	}

	// Fallback to major/minor parsing.
	major := strings.TrimPrefix(v.Major, "v")
	minorDigits := digitsPrefix(v.Minor)
	if major == "" || minorDigits == "" {
		return "", false
	}

	s = fmt.Sprintf("v%s.%s.0", major, minorDigits)
	if semver.IsValid(s) {
		return s, true
	}

	return "", false
}

func digitsPrefix(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}
