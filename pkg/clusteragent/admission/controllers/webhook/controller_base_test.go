// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package webhook

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewController(t *testing.T) {
	client := fake.NewSimpleClientset()
	wmeta := fxutil.Test[workloadmeta.Component](t, core.MockBundle(), workloadmeta.MockModule(), fx.Supply(workloadmeta.NewParams()))
	factory := informers.NewSharedInformerFactory(client, time.Duration(0))

	// V1
	controller := NewController(
		client,
		factory.Core().V1().Secrets(),
		factory.Admissionregistration(),
		func() bool { return true },
		make(chan struct{}),
		v1Cfg,
		wmeta,
		nil,
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
		wmeta,
		nil,
	)

	assert.IsType(t, &ControllerV1beta1{}, controller)
}
