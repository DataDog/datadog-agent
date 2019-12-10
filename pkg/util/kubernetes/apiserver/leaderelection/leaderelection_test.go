// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package leaderelection

import (
	"github.com/segmentio/encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	rl "k8s.io/client-go/tools/leaderelection/resourcelock"

	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
)

func makeLeaderCM(name, namespace, leaderIdentity string, leaseDuration int) *v1.ConfigMap {
	record := rl.LeaderElectionRecord{
		HolderIdentity:       leaderIdentity,
		LeaseDurationSeconds: leaseDuration,
		AcquireTime:          metav1.NewTime(time.Now()),
		RenewTime:            metav1.NewTime(time.Now().Add(time.Duration(leaseDuration) * time.Second)),
		LeaderTransitions:    1,
	}
	b, _ := json.Marshal(&record)

	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"control-plane.alpha.kubernetes.io/leader": string(b),
			},
		},
	}
}

type testSuite struct {
	suite.Suite
}

func (s *testSuite) TestError() {
	_, err := GetLeaderEngine()
	require.NotNil(s.T(), err)
}

func TestSuite(t *testing.T) {
	s := &testSuite{}
	suite.Run(t, s)
}

// TestNewLeaseAcquiring only test the proper creation of the lock,
// the acquisition of the leadership and that the ConfigMap contains is properly updated.
// The leadership transition is tested as part of an end to end test.
func TestNewLeaseAcquiring(t *testing.T) {
	const leaseName = "datadog-leader-election"

	client := fake.NewSimpleClientset()

	le := &LeaderEngine{
		HolderIdentity:  "foo",
		LeaseName:       leaseName,
		LeaderNamespace: "default",
		LeaseDuration:   1 * time.Second,

		coreClient: client.CoreV1(),
	}
	_, err := client.CoreV1().ConfigMaps("default").Get(leaseName, metav1.GetOptions{})
	require.True(t, errors.IsNotFound(err))

	le.leaderElector, err = le.newElection()
	require.NoError(t, err)

	newCm, err := client.CoreV1().ConfigMaps("default").Get(leaseName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, newCm.Name, leaseName)
	require.Nil(t, newCm.Annotations)

	le.EnsureLeaderElectionRuns()
	Cm, err := client.CoreV1().ConfigMaps("default").Get(leaseName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Contains(t, Cm.Annotations[rl.LeaderElectionRecordAnnotationKey], "\"leaderTransitions\":1")
	require.True(t, le.IsLeader())

	// As a leader, GetLeaderIP should return an empty IP
	ip, err := le.GetLeaderIP()
	assert.Equal(t, "", ip)
	assert.NoError(t, err)
}

func TestGetLeaderIPFollower(t *testing.T) {
	const leaseName = "datadog-leader-election"
	const endpointsName = "datadog-cluster-agent"

	client := fake.NewSimpleClientset()

	le := &LeaderEngine{
		HolderIdentity:  "foo",
		LeaseName:       leaseName,
		LeaderNamespace: "default",
		LeaseDuration:   120 * time.Second,
		coreClient:      client.CoreV1(),
	}

	// Create leader-election configmap with current node as follower
	electionCM := makeLeaderCM(leaseName, "default", "bar", 120)
	_, err := client.CoreV1().ConfigMaps("default").Create(electionCM)
	require.NoError(t, err)

	// Create endpoints
	endpoints := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      endpointsName,
			Namespace: "default",
		},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{
					{
						IP: "1.1.1.1",
						TargetRef: &v1.ObjectReference{
							Kind:      "pod",
							Namespace: "default",
							Name:      "foo",
						},
					},
					{
						IP: "1.1.1.2",
						TargetRef: &v1.ObjectReference{
							Kind:      "pod",
							Namespace: "default",
							Name:      "bar",
						},
					},
				},
			},
		},
	}
	stored_endpoints, err := client.CoreV1().Endpoints("default").Create(endpoints)
	require.NoError(t, err)

	// Run leader election
	le.leaderElector, err = le.newElection()
	require.NoError(t, err)
	le.EnsureLeaderElectionRuns()
	cm, err := client.CoreV1().ConfigMaps("default").Get(leaseName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Contains(t, cm.Annotations[rl.LeaderElectionRecordAnnotationKey], "\"leaderTransitions\":1")

	// We should be follower, and GetLeaderIP should return bar's IP
	require.False(t, le.IsLeader())
	ip, err := le.GetLeaderIP()
	assert.Equal(t, "1.1.1.2", ip)
	assert.NoError(t, err)

	// Remove bar from endpoints
	stored_endpoints.Subsets[0].Addresses = stored_endpoints.Subsets[0].Addresses[0:1]
	_, err = client.CoreV1().Endpoints("default").Update(stored_endpoints)
	require.NoError(t, err)

	// GetLeaderIP will "gracefully" error out
	ip, err = le.GetLeaderIP()
	assert.Equal(t, "", ip)
	assert.True(t, dderrors.IsNotFound(err))
}
