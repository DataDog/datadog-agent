// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package k8sconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const eksProcTable = `
kubelet --config /etc/kubernetes/kubelet/kubelet-config.json --kubeconfig /var/lib/kubelet/kubeconfig --container-runtime-endpoint unix:///run/containerd/containerd.sock --image-credential-provider-config /etc/eks/image-credential-provider/config.json --image-credential-provider-bin-dir /etc/eks/image-credential-provider --node-ip=192.168.78.181 --pod-infra-container-image=602401143452.dkr.ecr.eu-west-3.amazonaws.com/eks/pause:3.5 --v=2 --cloud-provider=aws --container-runtime=remote --node-labels=eks.amazonaws.com/sourceLaunchTemplateVersion=1,alpha.eksctl.io/cluster-name=PierreGuilleminotGravitonSandbox,alpha.eksctl.io/nodegroup-name=standard,eks.amazonaws.com/nodegroup-image=ami-09f37ddb4a6ecc85e,eks.amazonaws.com/capacityType=ON_DEMAND,eks.amazonaws.com/nodegroup=standard,eks.amazonaws.com/sourceLaunchTemplateId=lt-0df2e04572534b928 --max-pods=17
`

// TODO(jinroh): use testdata files
var eksFs = []*mockFile{
	{
		name: "/etc/eks/image-credential-provider",
		mode: 0755, isDir: true,
	},
	{
		name:    "/etc/eks/image-credential-provider/config.json",
		mode:    0644,
		content: `{\n  "apiVersion": "kubelet.config.k8s.io/v1alpha1",\n  "kind": "CredentialProviderConfig",\n  "providers": [\n    {\n      "name": "ecr-credential-provider",\n      "matchImages": [\n        "*.dkr.ecr.*.amazonaws.com",\n        "*.dkr.ecr.*.amazonaws.com.cn",\n        "*.dkr.ecr-fips.*.amazonaws.com",\n        "*.dkr.ecr.us-iso-east-1.c2s.ic.gov",\n        "*.dkr.ecr.us-isob-east-1.sc2s.sgov.gov"\n      ],\n      "defaultCacheDuration": "12h",\n      "apiVersion": "credentialprovider.kubelet.k8s.io/v1alpha1"\n    }\n  ]\n}`,
	},
	{
		name:    "/etc/eks/release",
		mode:    0664,
		content: `BASE_AMI_ID="ami-0528ac959959021be"\nBUILD_TIME="Sat May 13 01:48:34 UTC 2023"\nBUILD_KERNEL="5.10.178-162.673.amzn2.aarch64"\nARCH="aarch64"`,
	},
	{
		name:    "/etc/kubernetes/kubelet/kubelet-config.json",
		mode:    0644,
		content: `{\n  "kind": "KubeletConfiguration",\n  "apiVersion": "kubelet.config.k8s.io/v1beta1",\n  "address": "0.0.0.0",\n  "authentication": {\n    "anonymous": {\n      "enabled": false\n    },\n    "webhook": {\n      "cacheTTL": "2m0s",\n      "enabled": true\n    },\n    "x509": {\n      "clientCAFile": "/etc/kubernetes/pki/ca.crt"\n    }\n  },\n  "authorization": {\n    "mode": "Webhook",\n    "webhook": {\n      "cacheAuthorizedTTL": "5m0s",\n      "cacheUnauthorizedTTL": "30s"\n    }\n  },\n  "clusterDomain": "cluster.local",\n  "hairpinMode": "hairpin-veth",\n  "readOnlyPort": 0,\n  "cgroupDriver": "systemd",\n  "cgroupRoot": "/",\n  "featureGates": {\n    "RotateKubeletServerCertificate": true,\n    "KubeletCredentialProviders": true\n  },\n  "protectKernelDefaults": true,\n  "serializeImagePulls": false,\n  "serverTLSBootstrap": true,\n  "tlsCipherSuites": [\n    "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",\n    "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",\n    "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305",\n    "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",\n    "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305",\n    "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",\n    "TLS_RSA_WITH_AES_256_GCM_SHA384",\n    "TLS_RSA_WITH_AES_128_GCM_SHA256"\n  ],\n  "clusterDNS": [\n    "10.100.0.10"\n  ],\n  "kubeAPIQPS": 10,\n  "kubeAPIBurst": 20,\n  "evictionHard": {\n    "memory.available": "100Mi",\n    "nodefs.available": "10%",\n    "nodefs.inodesFree": "5%"\n  },\n  "kubeReserved": {\n    "cpu": "70m",\n    "ephemeral-storage": "1Gi",\n    "memory": "442Mi"\n  },\n  "systemReservedCgroup": "/system",\n  "kubeReservedCgroup": "/runtime"\n}`,
	},
	{
		name:    "/etc/systemd/system/kubelet.service.d/10-kubelet-args.conf",
		mode:    0644,
		content: `[Service]\nEnvironment='KUBELET_ARGS=--node-ip=192.168.78.181 --pod-infra-container-image=602401143452.dkr.ecr.eu-west-3.amazonaws.com/eks/pause:3.5 --v=2 --cloud-provider=aws --container-runtime=remote'`,
	},
	{
		name:    "/var/lib/kubelet/kubeconfig",
		mode:    0644,
		content: `apiVersion: v1\nkind: Config\nclusters:\n- cluster:\n    certificate-authority: /etc/kubernetes/pki/ca.crt\n    server: https://1DB2F34ED30B77AFEA800D56D3EBED0B.sk1.eu-west-3.eks.amazonaws.com\n  name: kubernetes\ncontexts:\n- context:\n    cluster: kubernetes\n    user: kubelet\n  name: kubelet\ncurrent-context: kubelet\nusers:\n- name: kubelet\n  user:\n    exec:\n      apiVersion: client.authentication.k8s.io/v1beta1\n      command: /usr/bin/aws-iam-authenticator\n      args:\n        - "token"\n        - "-i"\n        - "PierreGuilleminotGravitonSandbox"\n        - --region\n        - "eu-west-3"`,
	},
}

const kubadmProcTable = `
kube-proxy --config=/var/lib/kube-proxy/config.conf --hostname-override=lima-k8s
kubelet --bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf --kubeconfig=/etc/kubernetes/kubelet.conf --config=/var/lib/kubelet/config.yaml --container-runtime-endpoint=unix:///run/containerd/containerd.sock --pod-infra-container-image=registry.k8s.io/pause:3.9
etcd --advertise-client-urls=https://192.168.5.15:2379 --cert-file=/etc/kubernetes/pki/etcd/server.crt --client-cert-auth=true --data-dir=/var/lib/etcd --experimental-initial-corrupt-check=true --experimental-watch-progress-notify-interval=5s --initial-advertise-peer-urls=https://192.168.5.15:2380 --initial-cluster=lima-k8s=https://192.168.5.15:2380 --key-file=/etc/kubernetes/pki/etcd/server.key --listen-client-urls=https://127.0.0.1:2379,https://192.168.5.15:2379 --listen-metrics-urls=http://127.0.0.1:2381 --listen-peer-urls=https://192.168.5.15:2380 --name=lima-k8s --peer-cert-file=/etc/kubernetes/pki/etcd/peer.crt --peer-client-cert-auth=true --peer-key-file=/etc/kubernetes/pki/etcd/peer.key --peer-trusted-ca-file=/etc/kubernetes/pki/etcd/ca.crt --snapshot-count=10000 --trusted-ca-file=/etc/kubernetes/pki/etcd/ca.crt
kube-controller-manager --allocate-node-cidrs=true --authentication-kubeconfig=/etc/kubernetes/controller-manager.conf --authorization-kubeconfig=/etc/kubernetes/controller-manager.conf --bind-address=127.0.0.1 --client-ca-file=/etc/kubernetes/pki/ca.crt --cluster-cidr=10.244.0.0/16 --cluster-name=kubernetes --cluster-signing-cert-file=/etc/kubernetes/pki/ca.crt --cluster-signing-key-file=/etc/kubernetes/pki/ca.key --controllers=*,bootstrapsigner,tokencleaner --kubeconfig=/etc/kubernetes/controller-manager.conf --leader-elect=true --requestheader-client-ca-file=/etc/kubernetes/pki/front-proxy-ca.crt --root-ca-file=/etc/kubernetes/pki/ca.crt --service-account-private-key-file=/etc/kubernetes/pki/sa.key --service-cluster-ip-range=10.96.0.0/12 --use-service-account-credentials=true
kube-scheduler --authentication-kubeconfig=/etc/kubernetes/scheduler.conf --authorization-kubeconfig=/etc/kubernetes/scheduler.conf --bind-address=127.0.0.1 --kubeconfig=/etc/kubernetes/scheduler.conf --leader-elect=true
kube-apiserver --audit-policy-file=/etc/kubernetes/audit-policy.yaml --audit-log-path=/var/log/kubernetes/audit/audit.log --advertise-address=192.168.5.15 --allow-privileged=true --authorization-mode=Node,RBAC --client-ca-file=/etc/kubernetes/pki/ca.crt --enable-admission-plugins=NodeRestriction --enable-bootstrap-token-auth=true --etcd-cafile=/etc/kubernetes/pki/etcd/ca.crt --etcd-certfile=/etc/kubernetes/pki/apiserver-etcd-client.crt --etcd-keyfile=/etc/kubernetes/pki/apiserver-etcd-client.key --etcd-servers=https://127.0.0.1:2379 --kubelet-client-certificate=/etc/kubernetes/pki/apiserver-kubelet-client.crt --kubelet-client-key=/etc/kubernetes/pki/apiserver-kubelet-client.key --kubelet-preferred-address-types=InternalIP,ExternalIP,Hostname --proxy-client-cert-file=/etc/kubernetes/pki/front-proxy-client.crt --proxy-client-key-file=/etc/kubernetes/pki/front-proxy-client.key --requestheader-allowed-names=front-proxy-client --requestheader-client-ca-file=/etc/kubernetes/pki/front-proxy-ca.crt --requestheader-extra-headers-prefix=X-Remote-Extra- --requestheader-group-headers=X-Remote-Group --requestheader-username-headers=X-Remote-User --secure-port=6443 --service-account-issuer=https://kubernetes.default.svc.cluster.local --service-account-key-file=/etc/kubernetes/pki/sa.pub --service-account-signing-key-file=/etc/kubernetes/pki/sa.key --service-cluster-ip-range=10.96.0.0/12 --tls-cert-file=/etc/kubernetes/pki/apiserver.crt --tls-private-key-file=/etc/kubernetes/pki/apiserver.key
`

func TestKubAdmConfigLoader(t *testing.T) {
	var table []proc
	for _, l := range strings.Split(kubadmProcTable, "\n") {
		if l == "" {
			continue
		}
		cmdline := strings.Fields(l)
		table = append(table, buildProc(cmdline[0], cmdline))
	}

	tmpDir := t.TempDir()
	conf := loadTestConfiguration(tmpDir, table)
	assert.Empty(t, conf.Errors)
}

func TestKubEksConfigLoader(t *testing.T) {
	var table []proc
	for _, l := range strings.Split(eksProcTable, "\n") {
		if l == "" {
			continue
		}
		cmdline := strings.Fields(l)
		table = append(table, buildProc(cmdline[0], cmdline))
	}
	tmpDir := t.TempDir()
	for _, f := range eksFs {
		f.create(t, tmpDir)
	}
	conf := loadTestConfiguration(tmpDir, table)
	b, _ := json.MarshalIndent(conf, "", "  ")
	assert.Empty(t, conf.Errors)
	assert.NotNil(t, conf.ManagedEnvironment)
	assert.Equal(t, conf.ManagedEnvironment.Name, "eks")
	assert.True(t, strings.HasPrefix(conf.ManagedEnvironment.Metadata.(string), `BASE_AMI_ID="ami-0528ac959959021be"`))
	fmt.Println(string(b))
}

func loadTestConfiguration(hostroot string, table []proc) *K8sNodeConfig {
	l := &loader{hostroot: hostroot}
	_, data := l.load(context.Background(), func(ctx context.Context) []proc {
		return table
	})
	return data
}

type mockFile struct {
	isDir   bool
	name    string
	mode    uint32
	content string
}

func (f *mockFile) create(t *testing.T, root string) {
	if f.isDir {
		if err := os.MkdirAll(filepath.Join(root, f.name), fs.FileMode(f.mode)); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(f.name)), fs.FileMode(0755)); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, f.name), []byte(strings.ReplaceAll(f.content, "\\n", "\n")), os.FileMode(f.mode)); err != nil {
			t.Fatal(err)
		}
	}
}
