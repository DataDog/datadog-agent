// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
)

func TestValidateProcessingFailure(t *testing.T) {
	processingErr := errors.New("sensitive-processing-error")
	for _, tt := range []struct {
		name  string
		allow bool
		err   error
	}{
		{name: "allowed", allow: true},
		{name: "policy denial"},
		{name: "processing failure", allow: true, err: processingErr},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := &admission.Request{Namespace: "test"}
			allowed, err := Validate(request, "test", func(*admission.Request, string, dynamic.Interface) (bool, error) {
				return tt.allow, tt.err
			}, nil)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.allow && tt.err == nil, allowed)
			require.Equal(t, tt.err != nil, request.ProcessingFailed)
		})
	}
}
