// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gui

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"testing"

	"net/http/cookiejar"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

const (
	agentAPIPort   = 5001
	guiPort        = 5002
	guiAPIEndpoint = "/agent/gui/intent"
	// Default IPC cert path on Linux (agent creates ipc_cert.pem next to auth_token).
	ipcCertPathLinux = "/etc/datadog-agent/ipc_cert.pem"
	ipcCertPathWin   = "C:\\ProgramData\\Datadog\\ipc_cert.pem"
)

// assertAgentsUseKey checks that all agents are using the given key.
func getGUIIntentToken(t *assert.CollectT, host *components.RemoteHost, authtoken string, ipcCertPath string) string {
	client, err := host.NewHTTPClientWithIPCCert(ipcCertPath)
	require.NoErrorf(t, err, "failed to create HTTP client with IPC cert")

	apiEndpoint := &url.URL{
		Scheme: "https",
		Host:   net.JoinHostPort("localhost", strconv.Itoa(agentAPIPort)),
		Path:   guiAPIEndpoint,
	}

	req, err := http.NewRequest(http.MethodGet, apiEndpoint.String(), nil)
	require.NoErrorf(t, err, "failed to fetch API from %s", apiEndpoint.String())

	req.Header.Set("Authorization", "Bearer "+authtoken)

	resp, err := client.Do(req)
	require.NoErrorf(t, err, "failed to fetch intent token from %s", apiEndpoint.String())
	defer resp.Body.Close()

	require.Equalf(t, http.StatusOK, resp.StatusCode, "unexpected status code for %s", apiEndpoint.String())

	url, err := io.ReadAll(resp.Body)
	require.NoErrorf(t, err, "failed to read response body from %s", apiEndpoint.String())

	return string(url)
}

// assertGuiIsAvailable checks that the Agent GUI server is up and running.
func getGUIClient(t *assert.CollectT, host *components.RemoteHost, authtoken string, ipcCertPath string) *http.Client {
	intentToken := getGUIIntentToken(t, host, authtoken, ipcCertPath)

	guiURL := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort("localhost", strconv.Itoa(guiPort)),
		Path:   "/auth",
		RawQuery: url.Values{
			"intent": {intentToken},
		}.Encode(),
	}

	jar, err := cookiejar.New(&cookiejar.Options{})
	require.NoError(t, err)

	guiClient := host.NewHTTPClient()
	guiClient.Jar = jar

	// Make the GET request
	resp, err := guiClient.Get(guiURL.String())
	require.NoErrorf(t, err, "failed to reach GUI at address %s", guiURL.String())
	require.Equalf(t, http.StatusOK, resp.StatusCode, "unexpected status code for %s", guiURL.String())
	defer resp.Body.Close()

	cookies := guiClient.Jar.Cookies(&guiURL)
	assert.NotEmpty(t, cookies)
	assert.Equal(t, cookies[0].Name, "accessToken", "GUI server didn't the accessToken cookie")

	// Assert redirection to "/"
	assert.Equal(t, fmt.Sprintf("http://%v", net.JoinHostPort("localhost", strconv.Itoa(guiPort)))+"/", resp.Request.URL.String(), "GUI auth endpoint didn't redirect to root endpoint")

	return guiClient
}

func checkStaticFiles(t *testing.T, client *http.Client, host *components.RemoteHost, installPath string) {

	var links []string
	var traverse func(*html.Node)

	guiURL := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort("localhost", strconv.Itoa(guiPort)),
		Path:   "/",
	}

	// Make the GET request
	resp, err := client.Get(guiURL.String())
	require.NoErrorf(t, err, "failed to reach GUI at address %s", guiURL.String())
	require.Equalf(t, http.StatusOK, resp.StatusCode, "unexpected status code for %s", guiURL.String())
	defer resp.Body.Close()

	doc, err := html.Parse(resp.Body)
	require.NoErrorf(t, err, "failed to parse HTML response from GUI at address %s", guiURL.String())

	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "link":
				for _, attr := range n.Attr {
					if attr.Key == "href" {
						links = append(links, attr.Val)
					}
				}
			case "script":
				for _, attr := range n.Attr {
					if attr.Key == "src" {
						links = append(links, attr.Val)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}

	traverse(doc)
	for _, link := range links {
		t.Logf("trying to reach asset %v", link)
		fullLink := fmt.Sprintf("http://%v/%v", net.JoinHostPort("localhost", strconv.Itoa(guiPort)), link)
		resp, err := client.Get(fullLink)
		assert.NoErrorf(t, err, "failed to reach GUI asset at address %s", fullLink)
		defer resp.Body.Close()
		assert.Equalf(t, http.StatusOK, resp.StatusCode, "unexpected status code for %s", fullLink)

		body, err := io.ReadAll(resp.Body)
		// We replace windows line break by linux so the tests pass on every OS
		bodyContent := strings.ReplaceAll(string(body), "\r\n", "\n")
		assert.NoErrorf(t, err, "failed to read content of GUI asset at address %s", fullLink)

		// retrieving the served file in the Agent insallation director, removing the "view/" prefix
		expectedBody, err := host.ReadFile(path.Join(installPath, "bin", "agent", "dist", "views", strings.TrimLeft(link, "view/")))
		// We replace windows line break by linux so the tests pass on every OS
		expectedBodyContent := strings.ReplaceAll(string(expectedBody), "\r\n", "\n")
		assert.NoErrorf(t, err, "unable to retrieve file %v in the expected served files", link)

		assert.Equalf(t, expectedBodyContent, bodyContent, "content of the file %v is not the same as expected", link)
	}
}

func checkPingEndpoint(t *testing.T, client *http.Client) {
	guiURL := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort("localhost", strconv.Itoa(guiPort)),
		Path:   "/agent/ping",
	}

	// Make the GET request
	resp, err := client.Post(guiURL.String(), "", nil)
	require.NoErrorf(t, err, "failed to reach GUI at address %s", guiURL.String())
	require.Equalf(t, http.StatusOK, resp.StatusCode, "unexpected status code for %s", guiURL.String())
	defer resp.Body.Close()
}
