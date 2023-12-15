// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build docker && kubeapiserver

package leaderelection

/*
The leader Election package shouldn't be used for something else than leader election.
The leader election spawn an endless go routine to acquire the lead.
*/

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
	log "github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	rl "k8s.io/client-go/tools/leaderelection/resourcelock"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/test/integration/utils"
)

const setupTimeout = time.Second * 10
const leaseMinVersion = "v1.14.0"

type apiserverSuite struct {
	suite.Suite
	kubeConfigPath string
	usingLease     bool
}

func getApiserverComposePath(version string) string {
	return fmt.Sprintf("/tmp/apiserver-compose-%s.yaml", version)
}

func generateApiserverCompose(version string) error {
	apiserverCompose, err := os.ReadFile("testdata/apiserver-compose.yaml")
	if err != nil {
		return err
	}

	newComposeFile := strings.Replace(string(apiserverCompose), "APIVERSION_PLACEHOLDER", version, -1)

	err = os.WriteFile(getApiserverComposePath(version), []byte(newComposeFile), os.ModePerm)
	if err != nil {
		return err
	}
	return nil
}

func TestSuiteAPIServer(t *testing.T) {
	tests := []struct {
		name                          string
		version                       string
		usingLease                    bool
		leaderElectionDefaultResource string
	}{
		{
			"test version 1.18",
			"v1.18.20",
			true,
			"",
		},
		{
			"test version 1.13",
			"v1.13.2",
			false,
			"",
		},
		{
			"test version 1.18 with config set to configmap",
			"v1.18.20",
			false,
			"configmap",
		},
		{
			"test version 1.18 with config set to lease",
			"v1.18.20",
			false,
			"lease",
		},
	}
	for _, tt := range tests {
		s := &apiserverSuite{
			usingLease: tt.usingLease,
		}

		err := generateApiserverCompose(tt.version)
		require.NoError(t, err)
		defer func() {
			os.Remove(getApiserverComposePath(tt.version))
		}()

		mockConfig := config.Mock(t)
		config.SetFeatures(t, config.Kubernetes)
		mockConfig.SetWithoutSource("leader_election_default_resource", tt.leaderElectionDefaultResource)

		// Start compose stack
		compose := &utils.ComposeConf{
			ProjectName: "kube_events",
			FilePath:    getApiserverComposePath(tt.version),
			Variables:   map[string]string{},
		}
		output, err := compose.Start()
		defer compose.Stop()
		require.Nil(t, err, string(output))

		// Init apiclient
		pwd, err := os.Getwd()
		require.Nil(t, err)
		s.kubeConfigPath = filepath.Join(pwd, "testdata", "kubeconfig.json")
		mockConfig.SetWithoutSource("kubernetes_kubeconfig_path", s.kubeConfigPath)
		_, err = os.Stat(s.kubeConfigPath)
		require.Nil(t, err, fmt.Sprintf("%v", err))

		suite.Run(t, s)
	}
}

func (suite *apiserverSuite) SetupTest() {
	leaderelection.ResetGlobalLeaderEngine()
	telemetryComponent.GetCompatComponent().Reset()

	tick := time.NewTicker(time.Millisecond * 500)
	timeout := time.NewTicker(setupTimeout)

	k8sConfig, err := clientcmd.BuildConfigFromFlags("", suite.kubeConfigPath)
	require.Nil(suite.T(), err)

	k8sConfig.Timeout = 400 * time.Millisecond

	coreClient, err := corev1.NewForConfig(k8sConfig)
	require.Nil(suite.T(), err)
	for {
		select {
		case <-timeout.C:
			require.FailNow(suite.T(), "timeout after %s", setupTimeout.String())

		case <-tick.C:
			_, err := coreClient.Pods("").List(context.TODO(), metav1.ListOptions{Limit: 1})
			if err == nil {
				return
			}
			log.Warnf("Could not list pods: %s", err)
		}
	}
}

func (suite *apiserverSuite) waitForLeaderName(le *leaderelection.LeaderEngine) {
	var leaderName string
	tick := time.NewTicker(time.Second * 1)
	t := time.Second * 60
	timeout := time.NewTicker(t)

	for {
		select {
		case <-tick.C:
			leaderName = le.GetLeader()
			if leaderName == le.HolderIdentity {
				log.Infof("Waiting for leader: leader is %q", leaderName)
				return
			}
			log.Infof("Waiting for leader: leader is %q", leaderName)
		case <-timeout.C:
			require.FailNow(suite.T(), "timeout after %s", t.String())
		}
	}
}

func (suite *apiserverSuite) getNewLeaderEngine(holderIdentity string) *leaderelection.LeaderEngine {
	leaderelection.ResetGlobalLeaderEngine()
	telemetryComponent.GetCompatComponent().Reset()

	leader := leaderelection.CreateGlobalLeaderEngine(context.Background())
	leader.HolderIdentity = holderIdentity
	leader.LeaseDuration = time.Second * 30

	_, err := leaderelection.GetLeaderEngine()
	require.Nil(suite.T(), err)
	return leader
}

func (suite *apiserverSuite) TestLeaderElectionMulti() {
	const baseIdentityName = "test-multi-"
	testCases := []struct {
		leaderEngine *leaderelection.LeaderEngine
		initDelay    time.Duration
	}{
		{
			leaderEngine: suite.getNewLeaderEngine(fmt.Sprintf("%s%d", baseIdentityName, 0)),
			initDelay:    time.Millisecond * 0,
		},
		{
			leaderEngine: suite.getNewLeaderEngine(fmt.Sprintf("%s%d", baseIdentityName, 1)),
			initDelay:    time.Second * 1,
		},
	}
	for i, testCase := range testCases {
		suite.T().Run(
			fmt.Sprintf("%s-%d", testCase.leaderEngine.HolderIdentity, i),
			func(t *testing.T) {
				time.Sleep(testCase.initDelay)
				err := testCase.leaderEngine.EnsureLeaderElectionRuns()
				require.Nil(t, err)
			},
		)
	}
	// We sleep here to make sure that all instances in testCases are properly running.
	time.Sleep(time.Second * 1)

	// Leader
	actualLeader := testCases[0].leaderEngine
	suite.waitForLeaderName(actualLeader)
	require.True(suite.T(), actualLeader.IsLeader())

	// Follower
	actualFollower := testCases[1].leaderEngine
	require.False(suite.T(), actualFollower.IsLeader())

	for i, testCase := range testCases {
		assert.Equal(suite.T(), fmt.Sprintf("%s%d", baseIdentityName, i), testCase.leaderEngine.HolderIdentity)
		assert.Equal(suite.T(), actualLeader.HolderIdentity, testCase.leaderEngine.GetLeader())
	}

	c, err := apiserver.GetAPIClient()
	require.NoError(suite.T(), err)

	if suite.usingLease {
		client := c.Cl.CoordinationV1()
		require.Nil(suite.T(), err)
		leasesList, err := client.Leases(metav1.NamespaceDefault).List(context.TODO(), metav1.ListOptions{})
		require.Nil(suite.T(), err)
		// 1 Lease
		require.Len(suite.T(), leasesList.Items, 1)
		lease := leasesList.Items[0]
		require.Equal(suite.T(), "datadog-leader-election", lease.Name)
		require.NotNil(suite.T(), lease.Spec.HolderIdentity)
		require.Equal(suite.T(), testCases[0].leaderEngine.HolderIdentity, *lease.Spec.HolderIdentity)
	} else {
		client := c.Cl.CoreV1()
		require.Nil(suite.T(), err)
		cmList, err := client.ConfigMaps(metav1.NamespaceDefault).List(context.TODO(), metav1.ListOptions{})
		require.Nil(suite.T(), err)
		// 1 ConfigMap
		require.Len(suite.T(), cmList.Items, 1)

		var leaderAnnotation string
		var found bool
		for _, cm := range cmList.Items {
			if cm.Name == "datadog-leader-election" {
				require.False(suite.T(), found, "only one configmap match")
				leaderAnnotation, found = cm.Annotations[rl.LeaderElectionRecordAnnotationKey]
				require.True(suite.T(), found)
			}
		}
		require.Nil(suite.T(), err)
		expectedMessage := fmt.Sprintf(`"holderIdentity":"%s"`, testCases[0].leaderEngine.HolderIdentity)
		assert.Contains(suite.T(), leaderAnnotation, expectedMessage)
	}
}
