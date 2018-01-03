// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubelet

package kubelet

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	log "github.com/cihub/seelog"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// dummyKubelet allows tests to mock a kubelet's responses
type dummyKubelet struct {
	Requests chan *http.Request
	PodsBody []byte
}

func newDummyKubelet(podListJSONPath string) (*dummyKubelet, error) {
	var kubelet *dummyKubelet
	if podListJSONPath == "" {
		kubelet = &dummyKubelet{Requests: make(chan *http.Request, 3)}
	} else {
		podlist, err := ioutil.ReadFile(podListJSONPath)
		if err != nil {
			return nil, err
		}
		kubelet = &dummyKubelet{
			Requests: make(chan *http.Request, 3),
			PodsBody: podlist,
		}
	}
	return kubelet, nil
}

func (d *dummyKubelet) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debugf("dummyKubelet received %s on %s", r.Method, r.URL.Path)
	d.Requests <- r
	switch r.URL.Path {
	case "/healthz":
		w.Write([]byte("ok"))
	case "/pods":
		if d.PodsBody != nil {
			s, err := w.Write(d.PodsBody)
			log.Debugf("dummyKubelet wrote %s bytes, err: %s", s, err)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (d *dummyKubelet) Start() (*httptest.Server, int, error) {
	ts := httptest.NewServer(d)
	kubeletURL, err := url.Parse(ts.URL)
	if err != nil {
		return nil, 0, err
	}
	kubeletPort, err := strconv.Atoi(kubeletURL.Port())
	if err != nil {
		return nil, 0, err
	}
	return ts, kubeletPort, nil
}

type KubeletTestSuite struct {
	suite.Suite
}

// Make sure globalKubeUtil is deleted before each test
func (suite *KubeletTestSuite) SetupTest() {
	globalKubeUtil = nil
}

func (suite *KubeletTestSuite) TestLocateKubeletHTTP() {
	kubelet, err := newDummyKubelet("")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(suite.T(), err)

	config.Datadog.SetDefault("kubernetes_kubelet_host", "localhost")
	config.Datadog.SetDefault("kubernetes_http_kubelet_port", kubeletPort)

	kubeutil, err := GetKubeUtil()
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), kubeutil)

	select {
	case r := <-kubelet.Requests:
		require.Equal(suite.T(), r.Method, "GET")
		require.Equal(suite.T(), r.URL.Path, "/healthz")
	case <-time.After(2 * time.Second):
		require.FailNow(suite.T(), "Timeout on receive channel")
	}
}

func (suite *KubeletTestSuite) TestGetLocalPodList() {
	kubelet, err := newDummyKubelet("./testdata/podlist_1.6.json")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(suite.T(), err)

	config.Datadog.SetDefault("kubernetes_kubelet_host", "localhost")
	config.Datadog.SetDefault("kubernetes_http_kubelet_port", kubeletPort)

	kubeutil, err := GetKubeUtil()
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), kubeutil)
	<-kubelet.Requests // Throwing away /healthz GET

	pods, err := kubeutil.GetLocalPodList()
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), pods)
	require.Len(suite.T(), pods, 4)

	select {
	case r := <-kubelet.Requests:
		require.Equal(suite.T(), r.Method, "GET")
		require.Equal(suite.T(), r.URL.Path, "/pods")
	case <-time.After(2 * time.Second):
		require.FailNow(suite.T(), "Timeout on receive channel")
	}
}

func (suite *KubeletTestSuite) TestGetNodeInfo() {
	kubelet, err := newDummyKubelet("./testdata/podlist_1.6.json")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(suite.T(), err)

	config.Datadog.SetDefault("kubernetes_kubelet_host", "localhost")
	config.Datadog.SetDefault("kubernetes_http_kubelet_port", kubeletPort)

	kubeutil, err := GetKubeUtil()
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), kubeutil)
	<-kubelet.Requests // Throwing away /healthz GET

	ip, name, err := kubeutil.GetNodeInfo()
	require.Nil(suite.T(), err)
	require.Equal(suite.T(), ip, "10.132.0.9")
	require.Equal(suite.T(), name, "hostname")

	select {
	case r := <-kubelet.Requests:
		require.Equal(suite.T(), r.Method, "GET")
		require.Equal(suite.T(), r.URL.Path, "/pods")
	case <-time.After(2 * time.Second):
		require.FailNow(suite.T(), "Timeout on receive channel")
	}
}

func (suite *KubeletTestSuite) TestGetPodForContainerID() {
	kubelet, err := newDummyKubelet("./testdata/podlist_1.6.json")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(suite.T(), err)

	config.Datadog.SetDefault("kubernetes_kubelet_host", "localhost")
	config.Datadog.SetDefault("kubernetes_http_kubelet_port", kubeletPort)

	kubeutil, err := GetKubeUtil()
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), kubeutil)
	<-kubelet.Requests // Throwing away /healthz GET

	// Empty container ID
	pod, err := kubeutil.GetPodForContainerID("")
	<-kubelet.Requests // Throwing away /pods GET
	require.Nil(suite.T(), pod)
	require.NotNil(suite.T(), err)
	require.Contains(suite.T(), err.Error(), "containerID is empty")

	// Invalid container ID
	pod, err = kubeutil.GetPodForContainerID("invalid")
	<-kubelet.Requests // Throwing away /pods GET
	require.Nil(suite.T(), pod)
	require.NotNil(suite.T(), err)
	require.Contains(suite.T(), err.Error(), "container invalid not found in podlist")

	// Valid container ID
	pod, err = kubeutil.GetPodForContainerID("docker://1ce04128b3cccd7de0ae383516c28e0fe35cbb093195a72661723bdc06934840")
	<-kubelet.Requests // Throwing away /pods GET
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), pod)
	require.Equal(suite.T(), pod.Metadata.Name, "kube-dns-1829567597-2xtct")
}

func TestKubeletTestSuite(t *testing.T) {
	suite.Run(t, new(KubeletTestSuite))
}
