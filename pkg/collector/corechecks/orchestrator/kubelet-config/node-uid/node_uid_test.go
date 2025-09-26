//go:build kubelet && orchestrator

package client

import (
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "testing"

    pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
    "github.com/stretchr/testify/require"
)

func TestGetNodeUID_SuccessWithToken(t *testing.T) {
    // start a test server that validates the Authorization header and returns a UID
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        auth := r.Header.Get("Authorization")
        if auth != "Bearer mytoken" {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("uid-12345\n"))
    }))
    defer srv.Close()

    cfg := pkgconfigsetup.Datadog()
    // point the client at our test server
    cfg.SetWithoutSource("cluster_agent.url", srv.URL)
    cfg.SetWithoutSource("cluster_agent.auth_token", "mytoken")
    // ensure no token file is set
    cfg.SetWithoutSource("cluster_agent.auth_token_file", "")

    c, err := NewFromConfig()
    require.NoError(t, err)

    uid, err := c.GetNodeUID("node1")
    require.NoError(t, err)
    require.Equal(t, "uid-12345", uid)
}

func TestGetNodeUID_SuccessWithTokenFile(t *testing.T) {
    // create a temp dir + token file
    dir := t.TempDir()
    tokenFile := filepath.Join(dir, "token")
    require.NoError(t, os.WriteFile(tokenFile, []byte("filetoken\n"), 0o600))

    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        auth := r.Header.Get("Authorization")
        if auth != "Bearer filetoken" {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("uid-file-1"))
    }))
    defer srv.Close()

    cfg := pkgconfigsetup.Datadog()
    cfg.SetWithoutSource("cluster_agent.url", srv.URL)
    cfg.SetWithoutSource("cluster_agent.auth_token", "")
    cfg.SetWithoutSource("cluster_agent.auth_token_file", tokenFile)

    c, err := NewFromConfig()
    require.NoError(t, err)

    uid, err := c.GetNodeUID("node2")
    require.NoError(t, err)
    require.Equal(t, "uid-file-1", uid)
}

func TestNewFromConfig_MissingURL(t *testing.T) {
    cfg := pkgconfigsetup.Datadog()
    cfg.SetWithoutSource("cluster_agent.url", "")
    cfg.SetWithoutSource("cluster_agent.auth_token", "token")

    _, err := NewFromConfig()
    require.Error(t, err)
}
package client
