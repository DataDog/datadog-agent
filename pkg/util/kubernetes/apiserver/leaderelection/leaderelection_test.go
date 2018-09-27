// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package leaderelection

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	rl "k8s.io/client-go/tools/leaderelection/resourcelock"
)

func newFakeLockObject(namespace, name, leaderIdentity string) *v1.ConfigMap {
	return &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind: "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Annotations: map[string]string{
				rl.LeaderElectionRecordAnnotationKey: fmt.Sprintf(`{"holderIdentity":"%s"}`, leaderIdentity),
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

}
