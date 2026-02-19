// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package leaderelection

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	coordinationv1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/api/core/v1"
	discv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	rl "k8s.io/client-go/tools/leaderelection/resourcelock"

	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
	cmLock "github.com/DataDog/datadog-agent/internal/third_party/client-go/tools/leaderelection/resourcelock"
	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func makeLeaderLease(name, namespace, leaderIdentity string, leaseDuration int) *coordinationv1.Lease {
	duration := int32(leaseDuration)
	acquiretime := metav1.NewMicroTime(time.Now())
	renewtime := metav1.NewMicroTime(time.Now().Add(time.Duration(leaseDuration) * time.Second))
	leasetransition := int32(1)
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &leaderIdentity,
			LeaseDurationSeconds: &duration,
			AcquireTime:          &acquiretime,
			RenewTime:            &renewtime,
			LeaseTransitions:     &leasetransition,
		},
	}
}

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

// TestNewLeaseAcquiring only tests the proper creation of the lock,
// the acquisition of the leadership and that the ConfigMap/Lease contains is properly updated.
// The leadership transition is tested as part of an end to end test.
func TestNewLeaseAcquiring(t *testing.T) {
	const leaseName = "datadog-leader-election"

	tests := []struct {
		name     string
		lockType string
	}{
		{
			name:     "ConfigMap",
			lockType: cmLock.ConfigMapsResourceLock,
		},
		{
			name:     "Lease",
			lockType: rl.LeasesResourceLock,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()

			le := &LeaderEngine{
				ctx:             context.Background(),
				HolderIdentity:  "foo",
				LeaseName:       leaseName,
				LeaderNamespace: "default",
				LeaseDuration:   1 * time.Second,
				coreClient:      client.CoreV1(),
				coordClient:     client.CoordinationV1(),
				leaderMetric:    &dummyGauge{},
				lockType:        tt.lockType,
			}

			// Specific lease checks
			switch tt.lockType {
			case cmLock.ConfigMapsResourceLock:
				_, err := client.CoreV1().ConfigMaps("default").Get(context.TODO(), leaseName, metav1.GetOptions{})
				require.True(t, errors.IsNotFound(err))
			case rl.LeasesResourceLock:
				_, err := client.CoordinationV1().Leases("default").Get(context.TODO(), leaseName, metav1.GetOptions{})
				require.True(t, errors.IsNotFound(err))
			}
			var err error
			le.leaderElector, err = le.newElection()
			require.NoError(t, err)

			// Specific lease checks
			switch tt.lockType {
			case cmLock.ConfigMapsResourceLock:
				newCm, err := client.CoreV1().ConfigMaps("default").Get(context.TODO(), leaseName, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, newCm.Name, leaseName)
				require.Nil(t, newCm.Annotations)
			case rl.LeasesResourceLock:
				newLease, err := client.CoordinationV1().Leases("default").Get(context.TODO(), leaseName, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, newLease.Name, leaseName)
				require.Nil(t, newLease.Annotations)
			}

			err = le.EnsureLeaderElectionRuns()
			require.NoError(t, err)

			// Specific lease checks
			switch tt.lockType {
			case cmLock.ConfigMapsResourceLock:
				Cm, err := client.CoreV1().ConfigMaps("default").Get(context.TODO(), leaseName, metav1.GetOptions{})
				require.NoError(t, err)
				require.Contains(t, Cm.Annotations[rl.LeaderElectionRecordAnnotationKey], "\"leaderTransitions\":1")
			case rl.LeasesResourceLock:
				lease, err := client.CoordinationV1().Leases("default").Get(context.TODO(), leaseName, metav1.GetOptions{})
				require.NoError(t, err)
				require.NotNil(t, lease.Spec.LeaseTransitions)
				require.Equal(t, int32(1), *lease.Spec.LeaseTransitions)
			}

			require.True(t, le.IsLeader())

			// As a leader, GetLeaderIP should return an empty IP
			ip, err := le.GetLeaderIP()
			assert.Equal(t, "", ip)
			assert.NoError(t, err)
		})
	}
}

func TestSubscribe(t *testing.T) {
	const leaseName = "datadog-leader-election"
	for nb, tc := range []struct {
		name         string
		lockType     string
		getTokenFunc func(client *fake.Clientset) error
	}{
		{
			"subscribe_config_map",
			cmLock.ConfigMapsResourceLock,
			func(client *fake.Clientset) error {
				_, err := client.CoreV1().ConfigMaps("default").Get(context.TODO(), leaseName, metav1.GetOptions{})
				t.Logf("2 %v", err)
				return err
			},
		},
		{
			"subscribe_lease",
			rl.LeasesResourceLock,
			func(client *fake.Clientset) error {
				_, err := client.CoordinationV1().Leases("default").Get(context.TODO(), leaseName, metav1.GetOptions{})
				t.Logf("2 %v", err)
				return err
			},
		},
	} {
		t.Run(fmt.Sprintf("case %d: %s", nb, tc.name), func(t *testing.T) {
			client := fake.NewSimpleClientset()
			ctx, cancel := context.WithCancel(context.Background())
			le := &LeaderEngine{
				ctx:             ctx,
				HolderIdentity:  "foo",
				LeaseName:       leaseName,
				LeaderNamespace: "default",
				LeaseDuration:   1 * time.Second,
				coreClient:      client.CoreV1(),
				coordClient:     client.CoordinationV1(),
				leaderMetric:    &dummyGauge{},
				lockType:        tc.lockType,
			}

			notif1, _ := le.Subscribe()
			notif2, _ := le.Subscribe()
			require.Len(t, le.subscribers, 2)

			err := tc.getTokenFunc(client)
			require.True(t, errors.IsNotFound(err))

			le.leaderElector, err = le.newElection()
			require.NoError(t, err)

			le.EnsureLeaderElectionRuns()
			require.True(t, le.IsLeader())

			err = tc.getTokenFunc(client)
			require.NoError(t, err)

			counter1, counter2 := 0, 0
			for {
				select {
				case <-notif1:
					counter1++
					require.Truef(t, le.IsLeader(), "Expected to be leader")
					if counter1 > 1 {
						require.Fail(t, "Received too many notifications")
						break
					}

				case <-notif2:
					counter2++
					require.Truef(t, le.IsLeader(), "Expected to be leader")
					if counter2 > 1 {
						require.Fail(t, "Received too many notifications")
						break
					}

				case <-time.After(5 * time.Second):
					require.Fail(t, "Waiting on leader notification timed out")
					break
				}

				if counter1 == 1 && counter2 == 1 {
					break
				}
			}

			// simulate leadership lease loss by cancelling leader election context
			cancel()

			counter1, counter2 = 0, 0
			for {
				select {
				case <-notif1:
					require.Falsef(t, le.IsLeader(), "Expected to be a follower")
					counter1++
					if counter1 > 1 {
						require.Fail(t, "Received too many notifications")
						return
					}

				case <-notif2:
					require.Falsef(t, le.IsLeader(), "Expected to be a follower")
					counter2++
					if counter2 > 1 {
						require.Fail(t, "Received too many notifications")
						return
					}

				case <-time.After(5 * time.Second):
					require.Fail(t, "Waiting on leader notification timed out")
					return
				}

				if counter1 == 1 && counter2 == 1 {
					break
				}
			}

		})
	}

}

func TestGetLeaderIPFollower_ConfigMap(t *testing.T) {
	const leaseName = "datadog-leader-election"
	const serviceName = "datadog-cluster-agent"

	testCases := []struct {
		name              string
		useEndpointSlices bool
		setupEndpoints    func(*testing.T, *fake.Clientset, string)
		removeLeader      func(*testing.T, *fake.Clientset, string)
	}{
		{
			name:              "with Endpoints",
			useEndpointSlices: false,
			setupEndpoints: func(t *testing.T, client *fake.Clientset, serviceName string) {
				endpoints := &v1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serviceName,
						Namespace: "default",
					},
					Subsets: []v1.EndpointSubset{
						{
							Addresses: []v1.EndpointAddress{
								{
									IP:        "1.1.1.1",
									TargetRef: &v1.ObjectReference{Kind: "Pod", Namespace: "default", Name: "foo"},
								},
								{
									IP:        "1.1.1.2",
									TargetRef: &v1.ObjectReference{Kind: "Pod", Namespace: "default", Name: "bar"},
								},
							},
						},
					},
				}
				_, err := client.CoreV1().Endpoints("default").Create(context.TODO(), endpoints, metav1.CreateOptions{})
				require.NoError(t, err)
			},
			removeLeader: func(t *testing.T, client *fake.Clientset, serviceName string) {
				storedEndpoints, err := client.CoreV1().Endpoints("default").Get(context.TODO(), serviceName, metav1.GetOptions{})
				require.NoError(t, err)
				storedEndpoints.Subsets[0].Addresses = storedEndpoints.Subsets[0].Addresses[0:1]
				_, err = client.CoreV1().Endpoints("default").Update(context.TODO(), storedEndpoints, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
		{
			name:              "with EndpointSlices",
			useEndpointSlices: true,
			setupEndpoints: func(t *testing.T, client *fake.Clientset, serviceName string) {
				nodeName := "test-node"
				endpointSlice := &discv1.EndpointSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serviceName + "-abc123",
						Namespace: "default",
						Labels:    map[string]string{apiserver.KubernetesServiceNameLabel: serviceName},
					},
					Endpoints: []discv1.Endpoint{
						{
							Addresses: []string{"1.1.1.1"},
							TargetRef: &v1.ObjectReference{Kind: "Pod", Namespace: "default", Name: "foo"},
							NodeName:  &nodeName,
						},
						{
							Addresses: []string{"1.1.1.2"},
							TargetRef: &v1.ObjectReference{Kind: "Pod", Namespace: "default", Name: "bar"},
							NodeName:  &nodeName,
						},
					},
				}
				_, err := client.DiscoveryV1().EndpointSlices("default").Create(context.TODO(), endpointSlice, metav1.CreateOptions{})
				require.NoError(t, err)
			},
			removeLeader: func(t *testing.T, client *fake.Clientset, serviceName string) {
				storedSlice, err := client.DiscoveryV1().EndpointSlices("default").Get(context.TODO(), serviceName+"-abc123", metav1.GetOptions{})
				require.NoError(t, err)
				storedSlice.Endpoints = storedSlice.Endpoints[0:1]
				_, err = client.DiscoveryV1().EndpointSlices("default").Update(context.TODO(), storedSlice, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewClientset()

			if tc.useEndpointSlices {
				cache.Cache.Set("useEndpointSlices", true, time.Hour)
				defer cache.Cache.Delete("useEndpointSlices")
			}

			le := &LeaderEngine{
				ctx:             context.Background(),
				HolderIdentity:  "foo",
				LeaseName:       leaseName,
				ServiceName:     serviceName,
				LeaderNamespace: "default",
				LeaseDuration:   120 * time.Second,
				coreClient:      client.CoreV1(),
				coordClient:     client.CoordinationV1(),
				discoveryClient: client.DiscoveryV1(),
				leaderMetric:    &dummyGauge{},
				lockType:        cmLock.ConfigMapsResourceLock,
			}

			// Create leader-election configmap with current node as follower
			electionCM := makeLeaderCM(leaseName, "default", "bar", 120)
			_, err := client.CoreV1().ConfigMaps("default").Create(context.TODO(), electionCM, metav1.CreateOptions{})
			require.NoError(t, err)

			tc.setupEndpoints(t, client, serviceName)

			// Run leader election
			le.leaderElector, err = le.newElection()
			require.NoError(t, err)
			err = le.EnsureLeaderElectionRuns()
			require.NoError(t, err)
			cm, err := client.CoreV1().ConfigMaps("default").Get(context.TODO(), leaseName, metav1.GetOptions{})
			require.NoError(t, err)
			require.Contains(t, cm.Annotations[rl.LeaderElectionRecordAnnotationKey], "\"leaderTransitions\":1")

			// We should be follower, and GetLeaderIP should return bar's IP
			require.False(t, le.IsLeader())
			ip, err := le.GetLeaderIP()
			assert.NoError(t, err)
			assert.Equal(t, "1.1.1.2", ip, "GetLeaderIP should return same result for both APIs")

			// Remove bar from endpoints/endpointslice and clear cache
			cache.Cache.Delete("ip://bar")
			tc.removeLeader(t, client, serviceName)

			// GetLeaderIP will "gracefully" error out
			// Same behavior expected for both APIs
			ip, err = le.GetLeaderIP()
			assert.Equal(t, "", ip, "GetLeaderIP should return empty when leader not found")
			assert.True(t, dderrors.IsNotFound(err), "GetLeaderIP should return NotFound error")
		})
	}
}

func TestGetLeaderIPFollower_Lease(t *testing.T) {
	const leaseName = "datadog-leader-election"
	const serviceName = "datadog-cluster-agent"

	testCases := []struct {
		name              string
		useEndpointSlices bool
		setupEndpoints    func(*testing.T, *fake.Clientset, string)
		removeLeader      func(*testing.T, *fake.Clientset, string)
	}{
		{
			name:              "with Endpoints",
			useEndpointSlices: false,
			setupEndpoints: func(t *testing.T, client *fake.Clientset, serviceName string) {
				endpoints := &v1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serviceName,
						Namespace: "default",
					},
					Subsets: []v1.EndpointSubset{
						{
							Addresses: []v1.EndpointAddress{
								{
									IP:        "1.1.1.1",
									TargetRef: &v1.ObjectReference{Kind: "Pod", Namespace: "default", Name: "foo"},
								},
								{
									IP:        "1.1.1.2",
									TargetRef: &v1.ObjectReference{Kind: "Pod", Namespace: "default", Name: "bar"},
								},
							},
						},
					},
				}
				_, err := client.CoreV1().Endpoints("default").Create(context.TODO(), endpoints, metav1.CreateOptions{})
				require.NoError(t, err)
			},
			removeLeader: func(t *testing.T, client *fake.Clientset, serviceName string) {
				storedEndpoints, err := client.CoreV1().Endpoints("default").Get(context.TODO(), serviceName, metav1.GetOptions{})
				require.NoError(t, err)
				storedEndpoints.Subsets[0].Addresses = storedEndpoints.Subsets[0].Addresses[0:1]
				_, err = client.CoreV1().Endpoints("default").Update(context.TODO(), storedEndpoints, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
		{
			name:              "with EndpointSlices",
			useEndpointSlices: true,
			setupEndpoints: func(t *testing.T, client *fake.Clientset, serviceName string) {
				nodeName := "test-node"
				endpointSlice := &discv1.EndpointSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serviceName + "-abc123",
						Namespace: "default",
						Labels:    map[string]string{apiserver.KubernetesServiceNameLabel: serviceName},
					},
					Endpoints: []discv1.Endpoint{
						{
							Addresses: []string{"1.1.1.1"},
							TargetRef: &v1.ObjectReference{Kind: "Pod", Namespace: "default", Name: "foo"},
							NodeName:  &nodeName,
						},
						{
							Addresses: []string{"1.1.1.2"},
							TargetRef: &v1.ObjectReference{Kind: "Pod", Namespace: "default", Name: "bar"},
							NodeName:  &nodeName,
						},
					},
				}
				_, err := client.DiscoveryV1().EndpointSlices("default").Create(context.TODO(), endpointSlice, metav1.CreateOptions{})
				require.NoError(t, err)
			},
			removeLeader: func(t *testing.T, client *fake.Clientset, serviceName string) {
				storedSlice, err := client.DiscoveryV1().EndpointSlices("default").Get(context.TODO(), serviceName+"-abc123", metav1.GetOptions{})
				require.NoError(t, err)
				storedSlice.Endpoints = storedSlice.Endpoints[0:1]
				_, err = client.DiscoveryV1().EndpointSlices("default").Update(context.TODO(), storedSlice, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewClientset()

			// Set up config for EndpointSlices if needed
			if tc.useEndpointSlices {
				cache.Cache.Set("useEndpointSlices", true, time.Hour)
				defer cache.Cache.Delete("useEndpointSlices")
			}

			le := &LeaderEngine{
				ctx:             context.Background(),
				HolderIdentity:  "foo",
				LeaseName:       leaseName,
				ServiceName:     serviceName,
				LeaderNamespace: "default",
				LeaseDuration:   120 * time.Second,
				coreClient:      client.CoreV1(),
				coordClient:     client.CoordinationV1(),
				discoveryClient: client.DiscoveryV1(),
				leaderMetric:    &dummyGauge{},
				lockType:        rl.LeasesResourceLock,
			}

			// Create leader-election lease with current node as follower
			electionLease := makeLeaderLease(leaseName, "default", "bar", 120)
			_, err := client.CoordinationV1().Leases("default").Create(context.TODO(), electionLease, metav1.CreateOptions{})
			require.NoError(t, err)

			tc.setupEndpoints(t, client, serviceName)

			// Run leader election
			le.leaderElector, err = le.newElection()
			require.NoError(t, err)
			err = le.EnsureLeaderElectionRuns()
			require.NoError(t, err)
			lease, err := client.CoordinationV1().Leases("default").Get(context.TODO(), leaseName, metav1.GetOptions{})
			require.NoError(t, err)
			require.NotNil(t, lease.Spec.LeaseTransitions)
			require.Equal(t, int32(1), *lease.Spec.LeaseTransitions)

			// We should be follower, and GetLeaderIP should return bar's IP
			require.False(t, le.IsLeader())
			ip, err := le.GetLeaderIP()
			assert.NoError(t, err)
			assert.Equal(t, "1.1.1.2", ip, "GetLeaderIP should return same result for both APIs")

			// Remove bar from endpoints/endpointslice and clear cache
			cache.Cache.Delete("ip://bar")
			tc.removeLeader(t, client, serviceName)

			// GetLeaderIP will "gracefully" error out
			// Same behavior expected for both APIs
			ip, err = le.GetLeaderIP()
			assert.Equal(t, "", ip, "GetLeaderIP should return empty when leader not found")
			assert.True(t, dderrors.IsNotFound(err), "GetLeaderIP should return NotFound error")
		})
	}
}

type dummyGauge struct{}

// Set does nothing

func (g *dummyGauge) Set(_ float64, _ ...string) {}

// Inc does nothing
func (g *dummyGauge) Inc(_ ...string) {}

// Dec does nothing
func (g *dummyGauge) Dec(_ ...string) {}

// Add does nothing
func (g *dummyGauge) Add(_ float64, _ ...string) {}

// Sub does nothing
func (g *dummyGauge) Sub(_ float64, _ ...string) {}

// Delete does nothing
func (g *dummyGauge) Delete(_ ...string) {}

// DeletePartialMatch does nothing
func (g *dummyGauge) DeletePartialMatch(_ map[string]string) {}

// WithValues does nothing
func (g *dummyGauge) WithValues(_ ...string) telemetryComponent.SimpleGauge { return nil }

// WithTags does nothing
func (g *dummyGauge) WithTags(_ map[string]string) telemetryComponent.SimpleGauge { return nil }
