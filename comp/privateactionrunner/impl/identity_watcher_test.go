// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package privateactionrunnerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

func makeSecretWithURN(urn string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "par-identity", Namespace: "default"},
		Data:       map[string][]byte{"urn": []byte(urn), "private_key": []byte("key")},
	}
}

func newTestRunner(t *testing.T) *PrivateActionRunner {
	return &PrivateActionRunner{
		restartCh: make(chan struct{}, 1),
		logger:    logmock.New(t),
	}
}

func TestHandleIdentitySecretUpdate_URNChanged_SendsRestart(t *testing.T) {
	p := newTestRunner(t)
	p.handleIdentitySecretUpdate(makeSecretWithURN("urn-old"), makeSecretWithURN("urn-new"))
	assert.Len(t, p.restartCh, 1, "expected restart signal when URN changes")
}

func TestHandleIdentitySecretUpdate_URNUnchanged_NoRestart(t *testing.T) {
	p := newTestRunner(t)
	p.handleIdentitySecretUpdate(makeSecretWithURN("urn-same"), makeSecretWithURN("urn-same"))
	assert.Len(t, p.restartCh, 0, "expected no restart signal when URN is unchanged")
}

func TestHandleIdentitySecretUpdate_OtherFieldChanged_NoRestart(t *testing.T) {
	p := newTestRunner(t)
	old := makeSecretWithURN("urn-same")
	newS := makeSecretWithURN("urn-same")
	newS.Data["private_key"] = []byte("rotated-key")
	p.handleIdentitySecretUpdate(old, newS)
	assert.Len(t, p.restartCh, 0, "expected no restart signal when only private_key changes")
}

func TestHandleIdentitySecretUpdate_AlreadyQueued_NoDuplicate(t *testing.T) {
	p := newTestRunner(t)
	p.handleIdentitySecretUpdate(makeSecretWithURN("urn-a"), makeSecretWithURN("urn-b"))
	p.handleIdentitySecretUpdate(makeSecretWithURN("urn-b"), makeSecretWithURN("urn-c"))
	assert.Len(t, p.restartCh, 1, "expected at most one queued restart")
}

func TestHandleIdentitySecretUpdate_InvalidObjects_NoPanic(t *testing.T) {
	p := newTestRunner(t)
	assert.NotPanics(t, func() {
		p.handleIdentitySecretUpdate("not-a-secret", makeSecretWithURN("urn"))
		p.handleIdentitySecretUpdate(makeSecretWithURN("urn"), 42)
		p.handleIdentitySecretUpdate(nil, nil)
	})
	assert.Len(t, p.restartCh, 0)
}
