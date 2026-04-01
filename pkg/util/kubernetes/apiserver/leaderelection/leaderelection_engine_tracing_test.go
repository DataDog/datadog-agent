// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package leaderelection

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	rl "k8s.io/client-go/tools/leaderelection/resourcelock"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/mocktracer"

	cmLock "github.com/DataDog/datadog-agent/internal/third_party/client-go/tools/leaderelection/resourcelock"
)

func TestGetCurrentLeaderLease_Span(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	holderID := "test-holder"
	client := fake.NewClientset(&coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-lease",
			Namespace: "default",
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity: &holderID,
		},
	})

	le := &LeaderEngine{
		ctx:             context.Background(),
		LeaseName:       "test-lease",
		LeaderNamespace: "default",
		coordClient:     client.CoordinationV1(),
		lockType:        rl.LeasesResourceLock,
	}

	leader, err := le.getCurrentLeaderLease()
	require.NoError(t, err)
	assert.Equal(t, "test-holder", leader)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "leader_election.get_lease", span.OperationName())
	assert.Equal(t, "lease", span.Tag("lock_type"))
}

func TestGetCurrentLeaderLease_Error_Span(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	// No lease exists, so Get will return NotFound
	client := fake.NewClientset()

	le := &LeaderEngine{
		ctx:             context.Background(),
		LeaseName:       "nonexistent-lease",
		LeaderNamespace: "default",
		coordClient:     client.CoordinationV1(),
		lockType:        rl.LeasesResourceLock,
	}

	_, err := le.getCurrentLeaderLease()
	require.Error(t, err)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "leader_election.get_lease", span.OperationName())
	assert.Equal(t, "lease", span.Tag("lock_type"))
	assert.NotNil(t, span.Tag("error"))
}

func TestGetCurrentLeaderConfigMap_Span(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	record := `{"holderIdentity":"cm-leader"}`
	client := fake.NewClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-lease",
			Namespace: "default",
			Annotations: map[string]string{
				rl.LeaderElectionRecordAnnotationKey: record,
			},
		},
	})

	le := &LeaderEngine{
		ctx:             context.Background(),
		LeaseName:       "test-lease",
		LeaderNamespace: "default",
		coreClient:      client.CoreV1(),
		lockType:        cmLock.ConfigMapsResourceLock,
	}

	leader, err := le.getCurrentLeaderConfigMap()
	require.NoError(t, err)
	assert.Equal(t, "cm-leader", leader)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "leader_election.get_lease", span.OperationName())
	assert.Equal(t, "configmap", span.Tag("lock_type"))
}

func TestCreateLeaderTokenIfNotExists_Lease_Span(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	client := fake.NewClientset()

	le := &LeaderEngine{
		ctx:             context.Background(),
		LeaseName:       "test-lease",
		LeaderNamespace: "default",
		coordClient:     client.CoordinationV1(),
		coreClient:      client.CoreV1(),
		lockType:        rl.LeasesResourceLock,
	}

	err := le.createLeaderTokenIfNotExists()
	require.NoError(t, err)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "leader_election.create_token", span.OperationName())
}

func TestCreateLeaderTokenIfNotExists_ConfigMap_Span(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	client := fake.NewClientset()

	le := &LeaderEngine{
		ctx:             context.Background(),
		LeaseName:       "test-lease",
		LeaderNamespace: "default",
		coordClient:     client.CoordinationV1(),
		coreClient:      client.CoreV1(),
		lockType:        cmLock.ConfigMapsResourceLock,
	}

	err := le.createLeaderTokenIfNotExists()
	require.NoError(t, err)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "leader_election.create_token", span.OperationName())
}
