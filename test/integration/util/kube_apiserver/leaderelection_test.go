// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker
// +build kubeapiserver

package kubernetes

import (
	//"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/leaderelection"

	//"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"

	rl "k8s.io/client-go/tools/leaderelection/resourcelock"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
)

type apiserverSuite struct {
	suite.Suite
	apiClient      *apiserver.APIClient
	kubeConfigPath string
}

func TestSuiteAPIServer(t *testing.T) {
	s := &apiserverSuite{}

	// Start compose stack
	compose, err := initAPIServerCompose()
	require.Nil(t, err)
	output, err := compose.Start()
	defer compose.Stop()
	require.Nil(t, err, string(output))

	// Init apiclient
	pwd, err := os.Getwd()
	require.Nil(t, err)
	s.kubeConfigPath = filepath.Join(pwd, "testdata", "kubeconfig.json")
	config.Datadog.Set("kubernetes_kubeconfig_path", s.kubeConfigPath)
	_, err = os.Stat(s.kubeConfigPath)
	require.Nil(t, err, fmt.Sprintf("%v", err))

	suite.Run(t, s)
}

func (suite *apiserverSuite) SetupTest() {

	tick := time.NewTicker(time.Millisecond * 500)
	timeout := time.NewTicker(setupTimeout)

	k8sconfig, err := clientcmd.BuildConfigFromFlags("", suite.kubeConfigPath)
	require.Nil(suite.T(), err)

	k8sconfig.Timeout = 400 * time.Millisecond

	coreClient, err := corev1.NewForConfig(k8sconfig)

	for {
		select {
		case <-timeout.C:
			require.FailNow(suite.T(), "timeout after %s", setupTimeout.String())

		case <-tick.C:
			_, err := coreClient.Pods("").List(metav1.ListOptions{Limit: 1})
			if err == nil {
				return
			}
			log.Warnf("Could not list pods: %s", err)
		}
	}
}

func (suite *apiserverSuite) waitForLeaderName(le *leaderelection.LeaderEngine) {
	var leaderName string
	tick := time.NewTicker(time.Millisecond * 500)
	timeout := time.NewTicker(time.Second * 20)

	for {
		select {
		case <-tick.C:
			leaderName = le.GetLeader()
			if leaderName != "" {
				log.Infof("leader is %s", leaderName)
				return
			}
		case <-timeout.C:
			require.FailNow(suite.T(), "timeout after %s", setupTimeout.String())
		}
	}
}

func (suite *apiserverSuite) TestLeaderElection() {
	le, err := leaderelection.GetLeaderEngine()
	require.Nil(suite.T(), err)
	le.StartLeaderElection()

	client, err := leaderelection.GetClient()

	epList, err := client.Endpoints(metav1.NamespaceDefault).List(metav1.ListOptions{})
	require.Nil(suite.T(), err)
	// Kubernetes service and the created endpoint for the LE
	require.Len(suite.T(), epList.Items, 2)

	suite.waitForLeaderName(le)

	epList, err = client.Endpoints(metav1.NamespaceDefault).List(metav1.ListOptions{})
	require.Nil(suite.T(), err)
	var leaderAnnotation string
	for _, ep := range epList.Items {
		spew.Dump(ep)
		if ep.Name == "datadog-leader-election" {
			leaderAnnotation = ep.Annotations[rl.LeaderElectionRecordAnnotationKey]
		}
	}
	require.Nil(suite.T(), err)
	expectedMessage := fmt.Sprintf("\"holderIdentity\":\"%s\"", le.HolderIdentity)

	assert.Contains(suite.T(), leaderAnnotation, expectedMessage)
}
