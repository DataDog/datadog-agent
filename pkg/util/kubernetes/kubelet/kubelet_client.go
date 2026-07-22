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
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/client-go/transport"
)

const apiServerQuery = "/api/v1/pods?fieldSelector=spec.nodeName=%s"

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

	useAPIServer  bool
	apiServerHost string
	nodeName      string
}

type kubeletClient struct {
	client     http.Client
	kubeletURL string
	config     *kubeletClientConfig
}

func newForConfig(config *kubeletClientConfig, timeout time.Duration) (*kubeletClient, error) {
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
			// Ignore failure in retrieving root CA as kubelet_tls_verify=false make the RootCAs parameter un-used.
			if tlsConfig.InsecureSkipVerify {
				log.Debugf("Failed to retrieve root certificate authority from path %s: %s. Ignoring error as kubelet_tls_verify=false", config.caPath, err)
			} else {
				return nil, err
			}
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
		timeout = 30 * time.Second
	}
	httpClient.Timeout = timeout

	return &kubeletClient{
		client:     httpClient,
		kubeletURL: fmt.Sprintf("%s://%s", config.scheme, config.baseURL),
		config:     config,
	}, nil
}

func (kc *kubeletClient) checkConnection(ctx context.Context) error {
	// override the timeout for the check only
	// this will not affect the timeout for the actual client at runtime.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, statusCode, err := kc.query(ctx, "/healthz")
	if err != nil {
		return err
	}

	// unauthorized error, we can reach it but it's likely a token error
	if statusCode == http.StatusUnauthorized {
		return fmt.Errorf("unauthorized to request test kubelet endpoint (/healthz) - token used: %t", kc.config.token != "")
	}

	return nil
}

func (kc *kubeletClient) queryWithResp(ctx context.Context, path string) (io.ReadCloser, error) {
	_, response, err := kc.rawQuery(ctx, kc.kubeletURL, path)

	if err != nil {
		return nil, err
	}

	return response.Body, nil
}

func (kc *kubeletClient) rawQuery(ctx context.Context, baseURL string, path string) (*http.Request, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s%s", baseURL, path), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to create new request: %w", err)
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
		return nil, nil, err
	}

	return req, response, nil
}

func (kc *kubeletClient) query(ctx context.Context, path string) ([]byte, int, error) {
	// Redirect pod list requests to the API server when `useAPIServer` is enabled
	u := kc.kubeletURL
	if kc.config.useAPIServer && path == kubeletPodPath {
		path = fmt.Sprintf(apiServerQuery, url.QueryEscape(kc.config.nodeName))
		u = kc.config.apiServerHost
	}

	req, response, err := kc.rawQuery(ctx, u, path)
	if err != nil {
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

func getKubeletClientConfig() (*kubeletClientConfig, error) {
	var kubeletToken string
	var err error

	kubeletUseAPIServer := pkgconfigsetup.Datadog().GetBool("kubelet_use_api_server")
	kubeletTLSVerify := pkgconfigsetup.Datadog().GetBool("kubelet_tls_verify")
	kubeletCAPath := pkgconfigsetup.Datadog().GetString("kubelet_client_ca")
	kubeletClientCertPath := pkgconfigsetup.Datadog().GetString("kubelet_client_crt")
	kubeletClientKeyPath := pkgconfigsetup.Datadog().GetString("kubelet_client_key")
	kubeletTokenPath := pkgconfigsetup.Datadog().GetString("kubelet_auth_token_path")

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

	if kubeletUseAPIServer {
		apiServerHost := os.Getenv("KUBERNETES_SERVICE_HOST")
		apiServerPort := os.Getenv("KUBERNETES_SERVICE_PORT_HTTPS")
		if apiServerHost == "" || apiServerPort == "" {
			return nil, errors.New("failed to determine API server host/port")
		}

		clientConfig.useAPIServer = true
		clientConfig.apiServerHost = "https://" + net.JoinHostPort(apiServerHost, apiServerPort)

		log.Infof("kubeletUseApiServer set to true, pod list queries will be sent to the apiserver at: %s/api/v1/pods", clientConfig.apiServerHost)
	}

	return &clientConfig, nil
}

func getKubeletClient(ctx context.Context) (*kubeletClient, error) {
	// Step 1: Get the client config
	clientConfig, err := getKubeletClientConfig()
	if err != nil {
		return nil, err
	}

	// Step 2: Get the connection infos (ips, hostnames, ports, path prefix)
	potentialHosts, httpsPort, httpPort, pathPrefix, err := getKubeletConnectionInfo()
	if err != nil {
		return nil, err
	}

	// Step 3: Create the kubelet client
	var httpsErr error
	var newKubeletClient *kubeletClient
	kubeletTimeout := 30 * time.Second

	// Checking HTTPS first if port available
	if httpsPort > 0 {
		newKubeletClient, httpsErr = checkGetKubeletClient(ctx, "https", httpsPort, pathPrefix, potentialHosts, clientConfig, kubeletTimeout)
		if httpsErr != nil {
			if httpPort <= 0 {
				return nil, httpsErr
			}

			log.Warnf("Impossible to reach Kubelet through HTTPS, fallback to HTTP, err=%s", httpsErr.Error())
		} else {
			return newKubeletClient, nil
		}
	}

	// Check HTTP now if port available
	var httpErr error
	if httpPort > 0 {
		newKubeletClient, httpErr = checkGetKubeletClient(ctx, "http", httpPort, pathPrefix, potentialHosts, clientConfig, kubeletTimeout)
		if httpErr != nil {
			log.Debug("Impossible to reach Kubelet through HTTP")
			return nil, errors.New("impossible to reach Kubelet with HTTP. Please check if your setup requires kubelet_tls_verify = false. Activate debug logs to see all attempts made")
		}

		if httpsErr != nil {
			log.Warn("Unable to access Kubelet through HTTPS - Using HTTP connection instead. Please check if your setup requires kubelet_tls_verify = false")
		}

		return newKubeletClient, nil
	}

	return nil, errors.New("Invalid Kubelet configuration: both HTTPS and HTTP ports are disabled")
}

func checkGetKubeletClient(ctx context.Context, scheme string, port int, prefix string, hosts *connectionInfo, clientConfig *kubeletClientConfig, timeout time.Duration) (*kubeletClient, error) {
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
		kubeClient, err = newForConfig(clientConfig, timeout)
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
		return kubeClient, nil
	}

	for _, host := range hosts.hostnames {
		clientConfig.baseURL = fmt.Sprintf("%s:%d%s", host, port, prefix)

		log.Debugf("Trying to reach Kubelet at: %s", clientConfig.baseURL)
		kubeClient, err = newForConfig(clientConfig, timeout)
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
		return kubeClient, nil
	}

	return nil, errors.New("Kubelet connection check failed")
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
