// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package webhook

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewController(t *testing.T) {
	client := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(client, time.Duration(0))

	// V1
	controller := NewController(
		client,
		factory.Core().V1().Secrets(),
		factory.Admissionregistration(),
		func() bool { return true },
		make(chan struct{}),
		v1Cfg,
	)

	assert.IsType(t, &ControllerV1{}, controller)

	// V1beta1
	controller = NewController(
		client,
		factory.Core().V1().Secrets(),
		factory.Admissionregistration(),
		func() bool { return true },
		make(chan struct{}),
		v1beta1Cfg,
	)

	assert.IsType(t, &ControllerV1beta1{}, controller)
}
