// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

package start

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clustermetadata"
	cm "github.com/DataDog/datadog-agent/pkg/clustermetadata"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetes "k8s.io/client-go/kubernetes"
)

const (
	metadataRingLeaseDuration = 40 * time.Second
	metadataRingInterval      = 10 * time.Second
	metadataRingRetention     = 2 * time.Hour
)

func metadataRingScope(lc fx.Lifecycle, cfg config.Component, wmeta workloadmeta.Component, taggerComp tagger.Component, ipc ipc.Component) (workloadmeta.PodWatchScope, workloadmeta.NodeSyncReporter, *clustermetadata.PeerServer) {
	if !cfg.GetBool("cluster_agent.metadata_ring.enabled") {
		return nil, nil, nil
	}

	podName, err := common.GetSelfPodName()
	if err != nil {
		log.Errorf("metadata ring disabled: cannot determine this pod's name: %v", err)
		return nil, nil, nil
	}
	podNamespace := namespace.GetResourcesNamespace()
	selfID := clustermetadata.MemberID(podNamespace, podName)

	manager := clustermetadata.NewLeaseManager(
		func() (kubernetes.Interface, error) {
			client, err := apiserver.GetAPIClient()
			if err != nil {
				return nil, err
			}
			return client.InformerCl, nil
		},
		selfPodIP,
		podNamespace,
		podName,
		metadataRingLeaseDuration,
		metadataRingRetention,
	)

	nodeSource := func(ctx context.Context) ([]string, error) {
		nodes := wmeta.ListKubernetesNodes()
		names := make([]string, 0, len(nodes))
		for _, node := range nodes {
			names = append(names, node.EntityID.ID)
		}
		return names, nil
	}

	controller := clustermetadata.NewRingController(
		manager,
		selfID,
		metadataRingInterval,
		nodeSource,
	)

	pool := &metadataPeerPool{
		controller: controller,
		selfID:     selfID,
		tlsConfig:  ipc.GetTLSClientConfig(),
		authToken:  ipc.GetAuthToken(),
		port:       cfg.GetInt("cluster_agent.cmd_port"),
	}

	store := clustermetadata.NewLocalStore(wmeta, taggerComp, controller, pool.peers)
	scope := clustermetadata.NewRingScope(controller)

	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go controller.Run(ctx)
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			pool.close()
			return nil
		},
	})

	log.Infof("metadata ring enabled: member %s in namespace %s", selfID, podNamespace)
	return scope, scope, clustermetadata.NewPeerServer(store)
}

// metadataPeerPool holds one gRPC connection per alive ring member and
// exposes them as cm.Store peers. Connections are created lazily on first
// use and closed when a member leaves the ring.
type metadataPeerPool struct {
	controller *clustermetadata.RingController
	selfID     string
	tlsConfig  *tls.Config
	authToken  string
	port       int

	mu    sync.Mutex
	conns map[string]*grpc.ClientConn
}

// peers returns the current peer handles. A member without a published pod
// address is skipped until it renews with one.
func (p *metadataPeerPool) peers() []cm.Store {
	state := p.controller.State()

	p.mu.Lock()
	defer p.mu.Unlock()

	live := make(map[string]bool, len(state.MemberInfos))
	peers := make([]cm.Store, 0, len(state.MemberInfos))
	for _, info := range state.MemberInfos {
		if info.Name == p.selfID || info.PodIP == "" {
			continue
		}
		live[info.Name] = true

		conn := p.conns[info.Name]
		if conn == nil {
			conn, err := grpc.NewClient(
				net.JoinHostPort(info.PodIP, strconv.Itoa(p.port)),
				grpc.WithTransportCredentials(credentials.NewTLS(p.tlsConfig)),
				grpc.WithPerRPCCredentials(bearerToken{token: p.authToken}),
			)
			if err != nil {
				log.Warnf("metadata ring: cannot create peer client for %s: %v", info.Name, err)
				continue
			}
			if p.conns == nil {
				p.conns = make(map[string]*grpc.ClientConn)
			}
			p.conns[info.Name] = conn
		}
		peers = append(peers, clustermetadata.NewPeerClient(conn))
	}

	// Members that left: close and drop their connections.
	for name, conn := range p.conns {
		if !live[name] {
			conn.Close()
			delete(p.conns, name)
		}
	}
	return peers
}

func (p *metadataPeerPool) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for name, conn := range p.conns {
		conn.Close()
		delete(p.conns, name)
	}
}

// bearerToken supplies the DCA auth token on every peer RPC: peers are in
// the same trust domain as node agents.
type bearerToken struct {
	token string
}

func (b bearerToken) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + b.token}, nil
}

func (b bearerToken) RequireTransportSecurity() bool { return true }

// selfPodIP resolves this pod's IP: the DD_POD_IP env var (downward API)
// first, then the pod object through the API server.
func selfPodIP() (string, error) {
	if ip := os.Getenv("DD_POD_IP"); ip != "" {
		return ip, nil
	}

	client, err := apiserver.GetAPIClient()
	if err != nil {
		return "", err
	}
	podName, err := common.GetSelfPodName()
	if err != nil {
		return "", err
	}
	pod, err := client.InformerCl.CoreV1().Pods(namespace.GetResourcesNamespace()).Get(
		context.Background(), podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("cannot look up own pod for its IP: %w", err)
	}
	if pod.Status.PodIP == "" {
		return "", fmt.Errorf("own pod %s has no IP yet", podName)
	}
	return pod.Status.PodIP, nil
}
