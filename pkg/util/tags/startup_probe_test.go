// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tags

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// Cluster Agent states the startup probes run against. Each uses the real Cluster Agent client.
const (
	dcaUp         = "up"         // answers every request
	dcaDown       = "down"       // nothing listens, connections are refused at once
	dcaWedged     = "wedged"     // accepts connections and never answers
	dcaBlackholed = "blackholed" // a non-routable address, connections never complete
	// dcaWedgedClusterID answers every request except the cluster ID, which it never answers.
	dcaWedgedClusterID = "wedgedClusterID"
)

// blackholeAddr is a TEST-NET-1 address (RFC 5737): nothing answers there, so a connection
// attempt waits for the dial timeout.
const blackholeAddr = "192.0.2.1:5005"

// fakeClusterAgent starts a Cluster Agent endpoint in the given state and returns its URL.
// onEKS sets what the Cluster Agent reports about the node and the cluster.
func fakeClusterAgent(t *testing.T, state string, onEKS bool) string {
	t.Helper()
	switch state {
	case dcaUp, dcaWedgedClusterID:
		kubelet := "v1.31.0-gke.1000"
		if onEKS {
			kubelet = "v1.31.0-eks-a1b2c3d"
		}
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/version":
				_, _ = w.Write([]byte(`{"Major":7,"Minor":60,"Patch":0}`))
			case r.URL.Path == "/api/v1/cluster/id" && state == dcaWedgedClusterID:
				<-r.Context().Done()
			case r.URL.Path == "/api/v1/cluster/id":
				_, _ = w.Write([]byte(`"94e43011-177b-11ea-a4fe-42010a8401d2"`))
			case r.URL.Path == "/api/v1/cluster/eks-identity" && onEKS:
				_, _ = w.Write([]byte(`{"cluster_arn":"arn:aws:eks:us-west-2:123456789012:cluster/orders"}`))
			case strings.HasPrefix(r.URL.Path, "/api/v1/tags/node/"):
				_, _ = w.Write([]byte(`{}`))
			case strings.HasPrefix(r.URL.Path, "/api/v1/info/node/"):
				_ = json.NewEncoder(w).Encode(map[string]string{"KubeletVersion": kubelet})
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(srv.Close)
		return srv.URL
	case dcaDown:
		l, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := l.Addr().String()
		l.Close()
		return "https://" + addr
	case dcaWedged:
		l, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		var conns []net.Conn
		go func() {
			for {
				c, err := l.Accept()
				if err != nil {
					return
				}
				conns = append(conns, c) // held open, never read or answered
			}
		}()
		t.Cleanup(func() {
			l.Close()
			for _, c := range conns {
				c.Close()
			}
		})
		return "https://" + l.Addr().String()
	case dcaBlackholed:
		// Where there is no route to the address, the dial fails at once and this state would
		// degrade to "down". Say so rather than pass a test that checks nothing.
		c, err := net.DialTimeout("tcp", blackholeAddr, 300*time.Millisecond)
		if err == nil {
			c.Close()
		}
		var netErr net.Error
		if err != nil && !(errors.As(err, &netErr) && netErr.Timeout()) {
			t.Skipf("%s is not a blackhole here, the dial fails at once: %v", blackholeAddr, err)
		}
		return "https://" + blackholeAddr
	}
	t.Fatalf("unknown Cluster Agent state %q", state)
	return ""
}

// setupStartupNode configures a node Agent (or an EKS Fargate sidecar) that talks to the
// Cluster Agent at url, the way it is configured in production.
func setupStartupNode(t *testing.T, url string, fargate bool) model.BuildableConfig {
	t.Helper()
	cfg := configmock.New(t)
	cfg.SetInTest("cluster_agent.enabled", true)
	cfg.SetInTest("cluster_agent.url", url)
	cfg.SetInTest("cluster_agent.auth_token", "01234567890123456789012345678901")
	cfg.SetInTest("kubernetes_kubelet_nodename", "node-1")
	ipcmock.New(t)
	if fargate {
		env.SetFeatures(t, env.EKSFargate)
	} else {
		env.SetFeatures(t, env.Kubernetes)
	}
	recordFlavor := flavor.GetFlavor()
	t.Cleanup(func() { flavor.SetFlavor(recordFlavor) })
	flavor.SetFlavor(flavor.DefaultAgent)
	return cfg
}

// TestStartupProbe measures how long GetStaticTagsSlice blocks Agent startup with the
// production budgets, for one node kind and Cluster Agent state given in the environment. It
// runs once per process, because the Cluster Agent client and the startup lookup are process
// wide, exactly as at Agent startup:
//
//	EKS_IDENTITY_PROBE=nonEKS/wedged go test -tags test -run TestStartupProbe ./pkg/util/tags/
//
// Node kinds are nonEKS, EKS and fargate. It prints one PROBE line.
func TestStartupProbe(t *testing.T) {
	spec := os.Getenv("EKS_IDENTITY_PROBE")
	if spec == "" {
		t.Skip("set EKS_IDENTITY_PROBE=<nonEKS|EKS|fargate>/<up|down|wedged|blackholed> to run")
	}
	kind, state, _ := strings.Cut(spec, "/")
	onEKS := kind != "nonEKS"
	cfg := setupStartupNode(t, fakeClusterAgent(t, state, onEKS), kind == "fargate")
	setStartupEKSEvidence(t, onEKS)

	start := time.Now()
	tags := GetStaticTagsSlice(context.TODO(), cfg)
	elapsed := time.Since(start)
	identity := false
	for _, tag := range tags {
		if strings.HasPrefix(tag, "eks_cluster_arn:") {
			identity = true
		}
	}
	fmt.Printf("PROBE %s %s %.1fs identity=%v\n", kind, state, elapsed.Seconds(), identity)
}
