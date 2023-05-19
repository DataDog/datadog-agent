// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package admission

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"

	v1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/api/admissionregistration/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	success = tryResult(iota)
	notSupported
	unknown
)

type tryResult uint8

// apiDiscovery is a local struct adapted to the agent retry package.
// It allow discovering the Admissionregistration group versions with a retrier.
type apiDiscovery struct {
	v1retrier      retry.Retrier
	v1beta1retrier retry.Retrier
	v1Lister       func(ctx context.Context, opts metav1.ListOptions) (*v1.MutatingWebhookConfigurationList, error)
	v1beta1Lister  func(ctx context.Context, opts metav1.ListOptions) (*v1beta1.MutatingWebhookConfigurationList, error)
}

func (a *apiDiscovery) tryV1() error {
	_, err := a.v1Lister(context.TODO(), metav1.ListOptions{})
	return err
}

func (a *apiDiscovery) tryV1beta1() error {
	_, err := a.v1beta1Lister(context.TODO(), metav1.ListOptions{})
	return err
}

func newAPIDiscovery(ctx ControllerContext, retryCount uint, retryDelay time.Duration) (*apiDiscovery, error) {
	discovery := &apiDiscovery{
		v1Lister:      ctx.Client.AdmissionregistrationV1().MutatingWebhookConfigurations().List,
		v1beta1Lister: ctx.Client.AdmissionregistrationV1beta1().MutatingWebhookConfigurations().List,
	}

	if err := discovery.v1retrier.SetupRetrier(&retry.Config{
		Name:          "AdmissionV1Discovery",
		AttemptMethod: discovery.tryV1,
		Strategy:      retry.RetryCount,
		RetryCount:    retryCount,
		RetryDelay:    retryDelay,
	}); err != nil {
		return nil, err
	}

	if err := discovery.v1beta1retrier.SetupRetrier(&retry.Config{
		Name:          "AdmissionV1beta1Discovery",
		AttemptMethod: discovery.tryV1beta1,
		Strategy:      retry.RetryCount,
		RetryCount:    retryCount,
		RetryDelay:    retryDelay,
	}); err != nil {
		return nil, err
	}

	return discovery, nil
}

func errToResult(err error) tryResult {
	if err == nil {
		return success
	}

	if apierrors.IsNotFound(err) {
		return notSupported
	}

	return unknown
}

func try(r *retry.Retrier, groupVersion string) tryResult {
	log.Debugf("Trying Group version %q", groupVersion)
	for {
		_ = r.TriggerRetry()
		switch r.RetryStatus() {
		case retry.OK:
			return success
		case retry.PermaFail:
			err := r.LastError()
			log.Infof("Stopped retrying %q, last err: %v", groupVersion, err)
			return errToResult(err)
		}
	}
}

// useAdmissionV1 discovers which admissionregistration version should be used between v1beta1 and v1.
// - It tries to list v1 objects
// - If it succeed, fast return true
// - If it fails, it retries 3 times then tries v1beta1
// - If v1beta1 succeed, return false
// - If both versions can't be reached, fallback to v1
// - It fallback to v1beta1 only when v1 is explicitly not supported (got not found error)
func useAdmissionV1(ctx ControllerContext) (bool, error) {
	discovery, err := newAPIDiscovery(ctx, 3, 1*time.Second)
	if err != nil {
		return false, err
	}

	resultV1 := try(&discovery.v1retrier, "admissionregistration.k8s.io/v1")
	if resultV1 == success {
		log.Info("Group version 'admissionregistration.k8s.io/v1' is available, using it")
		return true, nil
	}

	resultV1beta1 := try(&discovery.v1beta1retrier, "admissionregistration.k8s.io/v1beta1")
	if resultV1beta1 == success {
		log.Info("Group version 'admissionregistration.k8s.io/v1beta' is available, using it")
		return false, nil
	}

	if resultV1 == notSupported && resultV1beta1 == unknown {
		// The only case where we want to fallback to v1beta1 is when v1 is explicitly not supported
		log.Info("Group version 'admissionregistration.k8s.io/v1' is not supported, falling back to 'v1beta'")
		return false, nil
	}

	// In case of no success in both versions, fallback to the newest (v1)
	log.Info("Falling back to 'admissionregistration.k8s.io/v1'")

	return true, nil
}
