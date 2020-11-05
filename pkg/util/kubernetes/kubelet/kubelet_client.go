// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubelet

package kubelet

import (
	"crypto/tls"
	"errors"
	"expvar"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	kubeletExpVar = expvar.NewInt("kubeletQueries")
)

type kubeletClientConfig struct {
	scheme         string
	baseURL        string
	tlsVerify      bool
	caPath         string
	clientCertPath string
	clientKeyPath  string
	token          string
}

type kubeletClient struct {
	client     http.Client
	kubeletURL string
	headers    http.Header
	config     kubeletClientConfig
}

func newForConfig(config kubeletClientConfig, timeout time.Duration) (*kubeletClient, error) {
	var err error

	// Building transport based on options
	customTransport := http.DefaultTransport.(*http.Transport).Clone()

	// Building custom TLS config
	tlsConfig := &tls.Config{}
	tlsConfig.InsecureSkipVerify = !config.tlsVerify

	if config.caPath == "" && filesystem.FileExists(kubernetes.DefaultServiceAccountCAPath) {
		config.caPath = kubernetes.DefaultServiceAccountCAPath
	}

	if config.caPath != "" {
		tlsConfig.RootCAs, err = kubernetes.GetCertificateAuthority(config.caPath)
		if err != nil {
			return nil, err
		}
	}

	if config.clientCertPath != "" && config.clientKeyPath != "" {
		tlsConfig.Certificates, err = kubernetes.GetCertificates(config.clientCertPath, config.clientKeyPath)
		if err != nil {
			return nil, err
		}
	}
	customTransport.TLSClientConfig = tlsConfig

	// Do not use token in plain text
	headers := http.Header{}
	if config.scheme == "https" {
		if config.token != "" {
			headers.Set(authorizationHeaderKey, fmt.Sprintf("bearer %s", config.token))
		}
	}

	// Defaulting timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &kubeletClient{
		client: http.Client{
			Transport: customTransport,
			Timeout:   timeout,
		},
		kubeletURL: fmt.Sprintf("%s://%s", config.scheme, config.baseURL),
		config:     config,
		headers:    headers,
	}, nil
}

func (kc *kubeletClient) checkConnection() error {
	_, statusCode, err := kc.query("/spec")
	if err != nil {
		return err
	}

	if statusCode == http.StatusUnauthorized {
		return fmt.Errorf("unauthorized to request test kubelet endpoint (/spec) - token used: %t", kc.headers.Get("Authorization") != "")
	}

	return nil
}

func (kc *kubeletClient) query(path string) ([]byte, int, error) {
	var err error

	req := &http.Request{}
	req.Header = kc.headers
	req.URL, err = url.Parse(fmt.Sprintf("%s%s", kc.kubeletURL, path))
	if err != nil {
		log.Debugf("Fail to create the kubelet request: %s", err)
		return nil, 0, err
	}

	response, err := kc.client.Do(req)
	kubeletExpVar.Add(1)
	if err != nil {
		log.Debugf("Cannot request %s: %s", req.URL.String(), err)
		return nil, 0, err
	}
	defer response.Body.Close()

	b, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Debugf("Fail to read request %s body: %s", req.URL.String(), err)
		return nil, 0, err
	}

	log.Tracef("Successfully queried %s, status code: %d, body len: %d", req.URL.String(), response.StatusCode, len(b))
	return b, response.StatusCode, nil
}

func getKubeletClient() (*kubeletClient, error) {
	var err error

	kubeletTimeout := 30 * time.Second
	kubeletProxyEnabled := config.Datadog.GetBool("eks_fargate")
	kubeletHost := config.Datadog.GetString("kubernetes_kubelet_host")
	kubeletHTTPSPort := config.Datadog.GetInt("kubernetes_https_kubelet_port")
	kubeletHTTPPort := config.Datadog.GetInt("kubernetes_http_kubelet_port")
	kubeletTLSVerify := config.Datadog.GetBool("kubelet_tls_verify")
	kubeletCAPath := config.Datadog.GetString("kubelet_client_ca")
	kubeletTokenPath := config.Datadog.GetString("kubelet_auth_token_path")
	kubeletClientCertPath := config.Datadog.GetString("kubelet_client_crt")
	kubeletClientKeyPath := config.Datadog.GetString("kubelet_client_key")
	kubeletNodeName := config.Datadog.Get("kubernetes_kubelet_nodename")
	var kubeletPathPrefix string
	var kubeletToken string

	// For some reason, token is not given as a path to Python part, so we need to read it here
	if kubeletTokenPath == "" && filesystem.FileExists(kubernetes.DefaultServiceAccountTokenPath) {
		kubeletTokenPath = kubernetes.DefaultServiceAccountTokenPath
	}

	if kubeletTokenPath != "" {
		kubeletToken, err = kubernetes.GetBearerToken(kubeletTokenPath)
		if err != nil {
			return nil, fmt.Errorf("kubelet token defined (%s) but unable to read, err: %w", kubeletTokenPath, err)
		}
	}

	clientConfig := kubeletClientConfig{
		tlsVerify:      kubeletTLSVerify,
		caPath:         kubeletCAPath,
		clientCertPath: kubeletClientCertPath,
		clientKeyPath:  kubeletClientKeyPath,
		token:          kubeletToken,
	}

	// Kubelet is unavailable, proxying calls through the APIServer (for instance EKS Fargate)
	var potentialHosts *connectionInfo
	if kubeletProxyEnabled {
		// Explicitly disable HTTP to reach APIServer
		kubeletHTTPPort = 0
		httpsPort, err := strconv.ParseInt(os.Getenv("KUBERNETES_SERVICE_PORT"), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("unable to get APIServer port: %w", err)
		}
		kubeletHTTPSPort = int(httpsPort)

		if config.Datadog.Get("kubernetes_kubelet_nodename") != "" {
			kubeletPathPrefix = fmt.Sprintf("/api/v1/nodes/%s/proxy", kubeletNodeName)
			apiServerHost := os.Getenv("KUBERNETES_SERVICE_HOST")

			potentialHosts = &connectionInfo{
				hostnames: []string{apiServerHost},
			}
			log.Infof("EKS on Fargate mode detected, will proxy calls to the Kubelet through the APIServer at %s:%d%s", apiServerHost, kubeletHTTPSPort, kubeletPathPrefix)
		} else {
			return nil, errors.New("kubelet proxy mode enabled but nodename is empty - unable to query")
		}
	} else {
		// Building a list of potential ips/hostnames to reach Kubelet
		potentialHosts = getPotentialKubeletHosts(kubeletHost)
	}

	// Checking HTTPS first if port available
	var httpsErr error
	if kubeletHTTPSPort > 0 {
		httpsErr = checkKubeletConnection("https", kubeletHTTPSPort, kubeletPathPrefix, potentialHosts, &clientConfig)
		if httpsErr != nil {
			log.Debug("Impossible to reach Kubelet through HTTPS")
			if kubeletHTTPPort <= 0 {
				return nil, httpsErr
			}
		} else {
			return newForConfig(clientConfig, kubeletTimeout)
		}
	}

	// Check HTTP now if port available
	var httpErr error
	if kubeletHTTPPort > 0 {
		httpErr = checkKubeletConnection("http", kubeletHTTPPort, kubeletPathPrefix, potentialHosts, &clientConfig)
		if httpErr != nil {
			log.Debug("Impossible to reach Kubelet through HTTP")
			return nil, fmt.Errorf("impossible to reach Kubelet with host: %s. Please check if your setup requires kubelet_tls_verify = false. Activate debug logs to see all attempts made", kubeletHost)
		}

		if httpsErr != nil {
			log.Warn("Unable to access Kubelet through HTTPS - Using HTTP connection instead. Please check if your setup requires kubelet_tls_verify = false")
		}

		return newForConfig(clientConfig, kubeletTimeout)
	}

	return nil, fmt.Errorf("Invalid Kubelet configuration: both HTTPS and HTTP ports are disabled")
}

func checkKubeletConnection(scheme string, port int, prefix string, hosts *connectionInfo, clientConfig *kubeletClientConfig) error {
	var err error
	var kubeClient *kubeletClient

	log.Debugf("Trying to reach Kubelet with scheme: %s", scheme)
	clientConfig.scheme = scheme

	for _, ip := range hosts.ips {
		clientConfig.baseURL = fmt.Sprintf("%s:%d", ip, port)

		log.Debugf("Trying to reach Kubelet at: %s", clientConfig.baseURL)
		kubeClient, err = newForConfig(*clientConfig, time.Second)
		if err != nil {
			log.Debugf("Failed to create Kubelet client for host: %s - error: %v", clientConfig.baseURL, err)
			continue
		}

		err = kubeClient.checkConnection()
		if err != nil {
			logConnectionError(clientConfig, err)
			continue
		}

		log.Infof("Successful configuration found for Kubelet, using URL: %s", kubeClient.kubeletURL)
		return nil
	}

	for _, host := range hosts.hostnames {
		clientConfig.baseURL = fmt.Sprintf("%s:%d%s", host, port, prefix)

		log.Debugf("Trying to reach Kubelet at: %s", clientConfig.baseURL)
		kubeClient, err = newForConfig(*clientConfig, time.Second)
		if err != nil {
			log.Debugf("Failed to create Kubelet client for host: %s - error: %v", clientConfig.baseURL, err)
			continue
		}

		err = kubeClient.checkConnection()
		if err != nil {
			logConnectionError(clientConfig, err)
			continue
		}

		log.Infof("Successful configuration found for Kubelet, using URL: %s", kubeClient.kubeletURL)
		return nil
	}

	return errors.New("Kubelet connection check failed")
}

func logConnectionError(clientConfig *kubeletClientConfig, err error) {
	switch {
	case strings.Contains(err.Error(), "x509: certificate is valid for"):
		log.Debugf(`Invalid x509 settings, the kubelet server certificate is not valid for this subject alternative name: %s, %v, Please check the SAN of the kubelet server certificate with "openssl x509 -in ${KUBELET_CERTIFICATE} -text -noout". `, clientConfig.baseURL, err)
	case strings.Contains(err.Error(), "x509: certificate signed by unknown authority"):
		log.Debugf(`The kubelet server certificate is signed by unknown authority, the current cacert is %s. Is the kubelet issuing self-signed certificates? Please validate the kubelet certificate with "openssl verify -CAfile %s ${KUBELET_CERTIFICATE}" to avoid this error: %v`, clientConfig.caPath, clientConfig.caPath, err)
	default:
		log.Debugf("Failed to reach Kubelet at: %s - error: %v", clientConfig.baseURL, err)
	}
}
