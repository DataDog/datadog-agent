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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
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

func TestStoppedLeading(t *testing.T) {
	const leaseName = "datadog-leader-election"

	client := fake.NewSimpleClientset()

	lock := newFakeLockObject("default", leaseName, "foo")
	_, err := client.CoreV1().ConfigMaps("default").Create(lock)
	require.NoError(t, err)

	le := &LeaderEngine{
		HolderIdentity:  "foo",
		LeaseName:       leaseName,
		LeaderNamespace: "default",
		LeaseDuration:   1 * time.Second,

		coreClient: client.CoreV1(),
	}

	le.leaderElector, err = le.newElection()
	require.NoError(t, err)

	le.EnsureLeaderElectionRuns()

	require.True(t, le.IsLeader())

	lock = newFakeLockObject("default", leaseName, "bar")
	_, err = client.CoreV1().ConfigMaps("default").Update(lock)
	require.NoError(t, err)

	// poll until election is lost
	err = wait.Poll(1*time.Second, 10*time.Second, func() (done bool, err error) {
		return le.IsLeader() == false, nil
	})
	require.NoError(t, err, "Should not be leader")

	lock = newFakeLockObject("default", leaseName, "foo")
	_, err = client.CoreV1().ConfigMaps("default").Update(lock)
	require.NoError(t, err)

	// poll until leader again
	err = wait.Poll(1*time.Second, 10*time.Second, func() (done bool, err error) {
		return le.IsLeader(), nil
	})
	require.NoError(t, err, "Should be leader")
}
