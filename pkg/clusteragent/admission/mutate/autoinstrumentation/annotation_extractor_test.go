// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"errors"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/stretchr/testify/require"
)

func TestAnnotationExtractor(t *testing.T) {
	var (
		identityExtractor = annotationExtractor[string]{
			key: "foo",
			do: func(in string) (string, error) {
				return in, nil
			},
		}
		validatingExtractor = annotationExtractor[string]{
			key: "foo",
			do: func(in string) (string, error) {
				if in != "bar" {
					return "", errors.New("invalid")
				}
				return in, nil
			},
		}
	)

	var testData = []struct {
		name        string
		annotations map[string]string
		extractor   annotationExtractor[string]
		err         bool
		ok          bool
		out         string
	}{
		{
			name:      "missing annotation",
			extractor: identityExtractor,
			ok:        false,
		},
		{
			name:        "key exists",
			extractor:   identityExtractor,
			ok:          true,
			out:         "bar",
			annotations: map[string]string{"foo": "bar"},
		},
		{
			name:        "errors work",
			extractor:   validatingExtractor,
			annotations: map[string]string{"foo": "nope"},
			err:         true,
			ok:          true,
		},
		{
			name: "transformers work & infallible",
			extractor: annotationExtractor[string]{
				key: "foo",
				do:  infallibleFn(strings.ToUpper),
			},
			annotations: map[string]string{"foo": "bar"},
			ok:          true,
			out:         "BAR",
		},
	}

	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			pod := common.FakePodSpec{Annotations: tt.annotations}.Create()
			data, err := tt.extractor.extract(pod)
			if tt.err {
				require.Error(t, err)
			} else if !tt.ok {
				require.True(t, isErrAnnotationNotFound(err))
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.out, data)
		})
	}
}
