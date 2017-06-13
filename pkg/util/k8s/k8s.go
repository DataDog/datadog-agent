package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/docker"
	log "github.com/cihub/seelog"
	"github.com/ericchiang/k8s"
	"github.com/ericchiang/k8s/api/v1"
)

// Kubelet constants
const (
	AuthTokenPath           = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	DefaultHTTPKubeletPort  = 10255
	DefaultHTTPSKubeletPort = 10250
	KubeletHealthPath       = "/healthz"
)

// Struct to hold the kubelet api url
// Instanciate with NewKubeUtil
type KubeUtil struct {
	kubeletAPIURL string
}

// NewKubeUtil returns a new instance of KubeUtil.
func NewKubeUtil(kubeletHost string, kubeletPort int) (*KubeUtil, error) {
	kubeletURL, err := locateKubelet(kubeletHost, kubeletPort)
	if err != nil {
		return nil, fmt.Errorf("Could not find a way to connect to kubelet: %s", err)
	}

	kubeUtil := KubeUtil{
		kubeletAPIURL: kubeletURL,
	}
	return &kubeUtil, nil
}

// GetNodeInfo returns the IP address and the hostname of the node where
// this pod is running.
func (ku *KubeUtil) GetNodeInfo() (ip, name string, err error) {
	pods, err := ku.GetLocalPodList()
	if err != nil {
		return "", "", fmt.Errorf("Error getting pod list from kubelet: %s", err)
	}

	for _, pod := range pods {
		if !pod.GetSpec().GetHostNetwork() {
			return pod.GetStatus().GetHostIP(), pod.GetSpec().GetHostname(), nil
		}
	}

	return "", "", fmt.Errorf("Failed to get node info")
}

// GetGlobalPodList returns the list of pods running on the cluster where this pod is running
// This function queries the API server which could put heavy load on it so use with caution
func GetGlobalPodList() ([]*v1.Pod, error) {
	client, err := k8s.NewInClusterClient()
	if err != nil {
		return nil, fmt.Errorf("Failed to get client: %s", err)
	}

	pods, err := client.CoreV1().ListPods(context.Background(), "")
	if err != nil {
		return nil, fmt.Errorf("Failed to get pods: %s", err)
	}

	return pods.GetItems(), nil
}

// GetLocalPodList returns the list of pods running on the node where this pod is running
func (ku *KubeUtil) GetLocalPodList() ([]*v1.Pod, error) {

	data, err := PerformKubeletQuery(fmt.Sprintf("%s/pods", ku.kubeletAPIURL))
	if err != nil {
		return nil, fmt.Errorf("Error performing kubelet query: %s", err)
	}

	var v *v1.PodList
	json.Unmarshal(data, v)

	return v.GetItems(), nil
}

// Try and find the hostname to query the kubelet
func locateKubelet(kubeletHost string, kubeletPort int) (string, error) {
	host := os.Getenv("KUBERNETES_KUBELET_HOST")
	var err error
	if host == "" {
		host = kubeletHost
	}
	if host == "" {
		host, err = docker.GetHostname()
		if err != nil {
			return "", fmt.Errorf("Unable to get hostname from docker: %s", err)
		}
	}

	port := kubeletPort
	if port == 0 {
		port = DefaultHTTPKubeletPort
	}

	url := fmt.Sprintf("http://%s:%s", host, port)
	if _, err := PerformKubeletQuery(url); err == nil {
		return url, nil
	}
	log.Debugf("Couldn't query kubelet over HTTP, assuming it's not in no_auth mode.")

	port = kubeletPort
	if port == 0 {
		port = DefaultHTTPSKubeletPort
	}

	url = fmt.Sprintf("https://%s:%s", host, port)
	if _, err := PerformKubeletQuery(url); err == nil {
		return url, nil
	}

	return "", fmt.Errorf("Could not find a method to connect to kubelet")
}

// PerformKubeletQuery performs a GET query against kubelet and return the response body
// Supports auth
func PerformKubeletQuery(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("Could not create request: %s", err)
	}

	if strings.HasPrefix(url, "https") {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", getAuthToken()))
	}

	res, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("Error executing request to %s: %s", url, err)
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("Error reading response from %s: %s", url, err)
	}
	return body, nil
}

// Read the kubelet token
func getAuthToken() string {
	token, err := ioutil.ReadFile(AuthTokenPath)
	if err != nil {
		log.Errorf("Could not read token from %s: %s", AuthTokenPath, err)
		return ""
	}
	return string(token)
}
