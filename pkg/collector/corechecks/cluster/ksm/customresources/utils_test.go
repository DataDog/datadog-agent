// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestObjectOwnerRef(t *testing.T) {
	tests := []struct {
		name     string
		owners   []metav1.OwnerReference
		wantKind string
		wantName string
		wantOk   bool
	}{
		{
			name:   "no owner references",
			owners: nil,
			wantOk: false,
		},
		{
			name: "single owner reference",
			owners: []metav1.OwnerReference{
				{Kind: "ScaledObject", Name: "my-scaledobject"},
			},
			wantKind: "scaledobject",
			wantName: "my-scaledobject",
			wantOk:   true,
		},
		{
			name: "multiple owner references returns the first",
			owners: []metav1.OwnerReference{
				{Kind: "ScaledObject", Name: "my-scaledobject"},
				{Kind: "SomeOtherKind", Name: "other-name"},
			},
			wantKind: "scaledobject",
			wantName: "my-scaledobject",
			wantOk:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &metav1.ObjectMeta{OwnerReferences: tt.owners}
			kind, name, ok := objectOwnerRef(obj)
			assert.Equal(t, tt.wantOk, ok)
			assert.Equal(t, tt.wantKind, kind)
			assert.Equal(t, tt.wantName, name)
		})
	}
}
