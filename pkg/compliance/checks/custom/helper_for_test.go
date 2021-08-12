// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package custom

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/scheme"
)

type kubeApiserverFixture struct {
	name         string
	checkFunc    CheckFunc
	objects      []runtime.Object
	expectReport *compliance.Report
	expectError  error
}

func (f *kubeApiserverFixture) run(t *testing.T) {
	t.Helper()

	assert := assert.New(t)

	env := &mocks.Env{}
	defer env.AssertExpectations(t)

	kubeClient := fake.NewSimpleDynamicClient(scheme.Scheme, f.objects...)
	env.On("KubeClient").Return(kubeClient)

	resource := compliance.Resource{
		Condition: "_",
		BaseResource: compliance.BaseResource{
			Custom: &compliance.Custom{
				Name: "customFunc",
			},
		},
	}
	expr, err := eval.ParseIterable(resource.Condition)
	assert.NoError(err)

	report, err := f.checkFunc(env, "rule-id", resource.Custom.Variables, expr)
	assert.Equal(f.expectReport, report)
	if f.expectError != nil {
		assert.EqualError(err, f.expectError.Error())
	} else {
		assert.NoError(err)
	}
}
