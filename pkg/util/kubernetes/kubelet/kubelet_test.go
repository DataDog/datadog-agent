// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

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

func TestLocateKubeletHTTP(t *testing.T) {
	kubelet, err := newDummyKubelet("")
	require.Nil(t, err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(t, err)

	config.Datadog.SetDefault("kubernetes_kubelet_host", "localhost")
	config.Datadog.SetDefault("kubernetes_http_kubelet_port", kubeletPort)

	kubeutil, err := NewKubeUtil()
	require.Nil(t, err)
	require.NotNil(t, kubeutil)

	select {
	case r := <-kubelet.Requests:
		require.Equal(t, r.Method, "GET")
		require.Equal(t, r.URL.Path, "/healthz")
	case <-time.After(2 * time.Second):
		require.FailNow(t, "Timeout on receive channel")
	}
}

func TestGetLocalPodList(t *testing.T) {
	kubelet, err := newDummyKubelet("./test/podlist_1.6.json")
	require.Nil(t, err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(t, err)

	config.Datadog.SetDefault("kubernetes_kubelet_host", "localhost")
	config.Datadog.SetDefault("kubernetes_http_kubelet_port", kubeletPort)

	kubeutil, err := NewKubeUtil()
	require.Nil(t, err)
	require.NotNil(t, kubeutil)
	<-kubelet.Requests // Throwing away /healthz GET

	pods, err := kubeutil.GetLocalPodList()
	require.Nil(t, err)
	require.NotNil(t, pods)
	require.Len(t, pods, 4)

	select {
	case r := <-kubelet.Requests:
		require.Equal(t, r.Method, "GET")
		require.Equal(t, r.URL.Path, "/pods")
	case <-time.After(2 * time.Second):
		require.FailNow(t, "Timeout on receive channel")
	}
}

func TestGetNodeInfo(t *testing.T) {
	kubelet, err := newDummyKubelet("./test/podlist_1.6.json")
	require.Nil(t, err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(t, err)

	config.Datadog.SetDefault("kubernetes_kubelet_host", "localhost")
	config.Datadog.SetDefault("kubernetes_http_kubelet_port", kubeletPort)

	kubeutil, err := NewKubeUtil()
	require.Nil(t, err)
	require.NotNil(t, kubeutil)
	<-kubelet.Requests // Throwing away /healthz GET

	ip, name, err := kubeutil.GetNodeInfo()
	require.Nil(t, err)
	require.Equal(t, ip, "10.132.0.9")
	require.Equal(t, name, "hostname")

	select {
	case r := <-kubelet.Requests:
		require.Equal(t, r.Method, "GET")
		require.Equal(t, r.URL.Path, "/pods")
	case <-time.After(2 * time.Second):
		require.FailNow(t, "Timeout on receive channel")
	}
}

func TestGetPodForContainerID(t *testing.T) {
	kubelet, err := newDummyKubelet("./test/podlist_1.6.json")
	require.Nil(t, err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(t, err)

	config.Datadog.SetDefault("kubernetes_kubelet_host", "localhost")
	config.Datadog.SetDefault("kubernetes_http_kubelet_port", kubeletPort)

	kubeutil, err := NewKubeUtil()
	require.Nil(t, err)
	require.NotNil(t, kubeutil)
	<-kubelet.Requests // Throwing away /healthz GET

	// Empty container ID
	pod, err := kubeutil.GetPodForContainerID("")
	<-kubelet.Requests // Throwing away /pods GET
	require.Nil(t, pod)
	require.NotNil(t, err)
	require.Contains(t, err.Error(), "containerID is empty")

	// Invalid container ID
	pod, err = kubeutil.GetPodForContainerID("invalid")
	<-kubelet.Requests // Throwing away /pods GET
	require.Nil(t, pod)
	require.NotNil(t, err)
	require.Contains(t, err.Error(), "container invalid not found in podlist")

	// Valid container ID
	pod, err = kubeutil.GetPodForContainerID("docker://1ce04128b3cccd7de0ae383516c28e0fe35cbb093195a72661723bdc06934840")
	<-kubelet.Requests // Throwing away /pods GET
	require.Nil(t, err)
	require.NotNil(t, pod)
	require.Equal(t, pod.Metadata.Name, "kube-dns-1829567597-2xtct")
}
