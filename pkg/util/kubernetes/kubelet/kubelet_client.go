// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"crypto/tls"
	"errors"
	"expvar"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/client-go/transport"
)

var (
	kubeletExpVar = expvar.NewInt("kubeletQueries")
	ipv6Re        = regexp.MustCompile(`^[0-9a-f:]+$`)
)

type kubeletClientConfig struct {
	scheme         string
	baseURL        string
	tlsVerify      bool
	caPath         string
	clientCertPath string
	clientKeyPath  string
	token          string
	tokenPath      string
}

type kubeletClient struct {
	client     http.Client
	kubeletURL string
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
	httpClient := http.Client{
		Transport: customTransport,
	}

	if config.scheme == "https" && config.token != "" {
		// Configure the authentication token.
		// Support dynamic auth tokens, aka Bound Service Account Token Volume (k8s v1.22+)
		// This uses the same refresh period used in the client-go (1 minute).
		httpClient.Transport, err = transport.NewBearerAuthWithRefreshRoundTripper(config.token, config.tokenPath, customTransport)
		if err != nil {
			return nil, err
		}
	}

	// Defaulting timeout
	if timeout == 0 {
		httpClient.Timeout = 30 * time.Second
	}

	return &kubeletClient{
		client:     httpClient,
		kubeletURL: fmt.Sprintf("%s://%s", config.scheme, config.baseURL),
		config:     config,
	}, nil
}

func (kc *kubeletClient) checkConnection(ctx context.Context) error {
	_, statusCode, err := kc.query(ctx, "/spec")
	if err != nil {
		return err
	}

	if statusCode == http.StatusUnauthorized {
		return fmt.Errorf("unauthorized to request test kubelet endpoint (/spec) - token used: %t", kc.config.token != "")
	}

	return nil
}

func (kc *kubeletClient) query(ctx context.Context, path string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s%s", kc.kubeletURL, path), nil)
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to create new request: %w", err)
	}

	response, err := kc.client.Do(req)
	kubeletExpVar.Add(1)

	// telemetry
	defer func() {
		code := 0
		if response != nil {
			code = response.StatusCode
		}

		queries.Inc(path, strconv.Itoa(code))
	}()

	if err != nil {
		log.Debugf("Cannot request %s: %s", req.URL.String(), err)
		return nil, 0, err
	}
	defer response.Body.Close()

	b, err := io.ReadAll(response.Body)
	if err != nil {
		log.Debugf("Fail to read request %s body: %s", req.URL.String(), err)
		return nil, 0, err
	}

	log.Tracef("Successfully queried %s, status code: %d, body len: %d", req.URL.String(), response.StatusCode, len(b))
	return b, response.StatusCode, nil
}

func getKubeletClient(ctx context.Context) (*kubeletClient, error) {
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
		tokenPath:      kubeletTokenPath,
	}

	// Kubelet is unavailable, proxying calls through the APIServer (for instance EKS Fargate)
	var potentialHosts *connectionInfo
	if kubeletProxyEnabled {
		// Explicitly disable HTTP to reach APIServer
		kubeletHTTPPort = 0
		httpsPort, err := strconv.ParseUint(os.Getenv("KUBERNETES_SERVICE_PORT"), 10, 16)
		if err != nil {
			return nil, fmt.Errorf("unable to get APIServer port: %w", err)
		}
		kubeletHTTPSPort = int(httpsPort)

		if config.Datadog.Get("kubernetes_kubelet_nodename") != "" {
			kubeletPathPrefix = fmt.Sprintf("/api/v1/nodes/%s/proxy", kubeletNodeName)
			apiServerIP := os.Getenv("KUBERNETES_SERVICE_HOST")

			potentialHosts = &connectionInfo{
				ips: []string{apiServerIP},
			}
			log.Infof("EKS on Fargate mode detected, will proxy calls to the Kubelet through the APIServer at %s:%d%s", apiServerIP, kubeletHTTPSPort, kubeletPathPrefix)
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
		httpsErr = checkKubeletConnection(ctx, "https", kubeletHTTPSPort, kubeletPathPrefix, potentialHosts, &clientConfig)
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
		httpErr = checkKubeletConnection(ctx, "http", kubeletHTTPPort, kubeletPathPrefix, potentialHosts, &clientConfig)
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

func checkKubeletConnection(ctx context.Context, scheme string, port int, prefix string, hosts *connectionInfo, clientConfig *kubeletClientConfig) error {
	var err error
	var kubeClient *kubeletClient

	log.Debugf("Trying to reach Kubelet with scheme: %s", scheme)
	clientConfig.scheme = scheme

	for _, ip := range hosts.ips {
		// If `ip` is an IPv6, it must be enclosed in square brackets
		if ipv6Re.MatchString(ip) {
			clientConfig.baseURL = fmt.Sprintf("[%s]:%d%s", ip, port, prefix)
		} else {
			clientConfig.baseURL = fmt.Sprintf("%s:%d%s", ip, port, prefix)
		}

		log.Debugf("Trying to reach Kubelet at: %s", clientConfig.baseURL)
		kubeClient, err = newForConfig(*clientConfig, time.Second)
		if err != nil {
			log.Debugf("Failed to create Kubelet client for host: %s - error: %v", clientConfig.baseURL, err)
			continue
		}

		err = kubeClient.checkConnection(ctx)
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

		err = kubeClient.checkConnection(ctx)
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
