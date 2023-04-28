// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package clusteragent

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	log "github.com/cihub/seelog"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubectl/pkg/scheme"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	admissioncmd "github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/api"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/custommetrics"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/api/healthprobe"
	admissionpkg "github.com/DataDog/datadog-agent/pkg/clusteragent/admission"
	admissionpatch "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	apicommon "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-agent/test/integration/utils"
	"golang.org/x/mod/semver"
)

const setupTimeout = time.Second * 10
const leaseMinVersion = "v1.14.0"
const admissionV1MinVersion = "v1.16.4"

type apiserverSuite struct {
	suite.Suite
	kubeConfigPath   string
	usingLease       bool
	usingAdmissionV1 bool
	mockConfig       *config.MockConfig
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

func (s *apiserverSuite) versions(apiServerVersion string) {
	// leases
	if semver.Compare(apiServerVersion, leaseMinVersion) < 0 {
		s.usingLease = false
	} else {
		s.usingLease = true
	}
	// admission controller
	if semver.Compare(apiServerVersion, admissionV1MinVersion) < 0 {
		s.usingAdmissionV1 = false
	} else {
		s.usingAdmissionV1 = true
	}

}

func TestSuiteAPIServer(t *testing.T) {
	tests := []struct {
		name                     string
		version                  string
		admissionExpectedVersion string
	}{
		{
			"test version 1.22",
			"v1.22.5",
			"v1",
		},
		{
			"test version 1.18",
			"v1.18.20",
			"v1",
		},
		{
			"test version 1.13",
			"v1.13.2",
			"v1beta1",
		},
	}
	for _, tt := range tests {
		require.True(t, semver.IsValid(tt.version))
		s := &apiserverSuite{
			mockConfig: config.Mock(t),
		}
		s.versions(tt.version)

		err := generateApiserverCompose(tt.version)
		require.NoError(t, err)
		defer func() {
			os.Remove(getApiserverComposePath(tt.version))
		}()

		config.SetFeatures(t, config.Kubernetes)

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
		s.mockConfig.Set("kubernetes_kubeconfig_path", s.kubeConfigPath)
		_, err = os.Stat(s.kubeConfigPath)
		require.Nil(t, err, fmt.Sprintf("%v", err))

		suite.Run(t, s)
	}
}

func (suite *apiserverSuite) SetupTest() {
	leaderelection.ResetGlobalLeaderEngine()
	telemetry.Reset()

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

func (suite *apiserverSuite) TestCanStart() {
	mainCtx, mainCtxCancel := context.WithCancel(context.Background())
	defer mainCtxCancel()

	// Starting Cluster Agent sequence
	// Initialization order is important for multiple reasons, see comments
	err := util.SetupCoreDump(suite.mockConfig)
	require.NoError(suite.T(), err)

	// Expose the registered metrics via HTTP.
	http.Handle("/metrics", telemetry.Handler())
	metricsPort := suite.mockConfig.GetInt("metrics_port")
	metricsServer := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", metricsPort),
		Handler: http.DefaultServeMux,
	}

	go func() {
		err := metricsServer.ListenAndServe()
		require.NoError(suite.T(), err)
	}()

	// Setup healthcheck port
	var healthPort = 3000
	if healthPort > 0 {
		err := healthprobe.Serve(mainCtx, healthPort)
		require.NoError(suite.T(), err)
	}

	// Starting server early to ease investigations
	err = api.StartServer()
	require.NoError(suite.T(), err)

	// Getting connection to APIServer, it's done before Hostname resolution
	// as hostname resolution may call APIServer
	apiCl, err := apiserver.WaitForAPIClient(context.Background()) // make sure we can connect to the apiserver
	require.NoError(suite.T(), err)

	// Get hostname as aggregator requires hostname
	hname, err := hostname.Get(mainCtx)
	require.NoError(suite.T(), err)

	// If a cluster-agent looses the connectivity to DataDog, we still want it to remain ready so that its endpoint remains in the service because:
	// * It is still able to serve metrics to the WPA controller and
	// * The metrics reported are reported as stale so that there is no "lie" about the accuracy of the reported metrics.
	// Serving stale data is better than serving no data at all.
	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.UseEventPlatformForwarder = false
	forwarder := fxutil.Test[defaultforwarder.Component](suite.T(), defaultforwarder.MockModule, defaultforwarder.MockModule)
	demux := aggregator.InitAndStartAgentDemultiplexer(forwarder, opts, hname)
	demux.AddAgentStartupTelemetry(fmt.Sprintf("%s - Datadog Cluster Agent", version.AgentVersion))

	le, err := leaderelection.GetLeaderEngine()
	require.NoError(suite.T(), err)

	// Create event recorder
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: apiCl.Cl.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "datadog-cluster-agent"})

	stopCh := make(chan struct{})
	ctx := apiserver.ControllerContext{
		InformerFactory:    apiCl.InformerFactory,
		WPAClient:          apiCl.WPAClient,
		WPAInformerFactory: apiCl.WPAInformerFactory,
		DDClient:           apiCl.DDClient,
		DDInformerFactory:  apiCl.DynamicInformerFactory,
		Client:             apiCl.Cl,
		IsLeaderFunc:       le.IsLeader,
		EventRecorder:      eventRecorder,
		StopCh:             stopCh,
	}

	if aggErr := apiserver.StartControllers(ctx); aggErr != nil {
		require.NoError(suite.T(), aggErr)

	}

	clusterName := clustername.GetRFC1123CompliantClusterName(context.TODO(), hname)
	// Generate and persist a cluster ID
	// this must be a UUID, and ideally be stable for the lifetime of a cluster,
	// so we store it in a configmap that we try and read before generating a new one.
	coreClient := apiCl.Cl.CoreV1().(*corev1.CoreV1Client)
	_, err = apicommon.GetOrCreateClusterID(coreClient)
	require.NoError(suite.T(), err)

	// create and setup the Autoconfig instance
	common.LoadComponents(mainCtx, suite.mockConfig.GetString("confd_path"))

	// Set up check collector
	common.AC.AddScheduler("check", collector.InitCheckScheduler(common.Coll), true)
	common.Coll.Start()

	// start the autoconfig, this will immediately run any configured check
	common.AC.LoadAndRun(mainCtx)

	// Start the cluster check Autodiscovery
	_, err = clusterchecks.NewHandler(common.AC)
	require.NoError(suite.T(), err)

	wg := sync.WaitGroup{}
	// Autoscaler Controller Goroutine
	// Start the k8s custom metrics server. This is a blocking call
	wg.Add(1)
	go func() {
		defer wg.Done()

		err := custommetrics.RunServer(mainCtx, apiCl)
		require.NoError(suite.T(), err)
	}()

	// Ignore compliance and start admission
	usingV1, err := admissionpkg.UseAdmissionV1(apiCl.DiscoveryCl)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), suite.usingAdmissionV1, usingV1)
	patchCtx := admissionpatch.ControllerContext{
		IsLeaderFunc:        le.IsLeader,
		LeaderSubscribeFunc: le.Subscribe,
		K8sClient:           apiCl.Cl,
		ClusterName:         clusterName,
		StopCh:              stopCh,
	}
	err = admissionpatch.StartControllers(patchCtx)
	require.NoError(suite.T(), err)

	admissionCtx := admissionpkg.ControllerContext{
		IsLeaderFunc:        le.IsLeader,
		LeaderSubscribeFunc: le.Subscribe,
		SecretInformers:     apiCl.CertificateSecretInformerFactory,
		WebhookInformers:    apiCl.WebhookConfigInformerFactory,
		Client:              apiCl.Cl,
		DiscoveryClient:     apiCl.DiscoveryCl,
		StopCh:              stopCh,
	}

	err = admissionpkg.StartControllers(admissionCtx)
	require.NoError(suite.T(), err)

	// Webhook and secret controllers are started successfully
	// Setup the the k8s admission webhook server
	server := admissioncmd.NewServer()

	// Start the k8s admission webhook server
	wg.Add(1)
	go func() {
		defer wg.Done()

		err := server.Run(mainCtx, apiCl.Cl)
		require.NoError(suite.T(), err)
	}()

	// retrieve the agent health before stopping the components
	// GetReadyNonBlocking has a 100ms timeout to avoid blocking
	_, err = health.GetReadyNonBlocking()
	require.NoError(suite.T(), err)

	// Cancel the main context to stop components
	mainCtxCancel()

	// wait for the External Metrics Server and the Admission Webhook Server to
	// stop properly
	wg.Wait()

	close(stopCh)

	demux.Stop(true)
	err = metricsServer.Shutdown(context.Background())
	require.NoError(suite.T(), err)
}
