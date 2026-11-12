// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cspm

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubecomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

// kubeletConfigDir is the drop-in directory given to the kubelet of the kind
// worker with --config-dir.
const kubeletConfigDir = "/etc/kubernetes/kubelet.conf.d"

type kubeletConfigDirTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// TestKubeletConfigDirCSPM runs a kind worker, a kubeadm node, whose kubelet
// reads drop-ins from kubeletConfigDir. The directory is mounted from the VM,
// where Docker creates it, so it exists when the kubelet starts.
func TestKubeletConfigDirCSPM(t *testing.T) {
	e2e.Run(t, &kubeletConfigDirTestSuite{}, e2e.WithProvisioner(
		provkindvm.Provisioner(
			provkindvm.WithRunOptions(
				scenkindvm.WithInternetAccess(),
				scenkindvm.WithAgentOptions(kubernetesagentparams.WithHelmValues(complianceValues)),
				scenkindvm.WithKindWorkerNodes(kubecomp.KindWorkerNode{
					KubeletExtraArgs: map[string]string{"config-dir": kubeletConfigDir},
					ExtraMounts:      []kubecomp.Mount{{HostPath: "/var/lib/e2e-kubelet.conf.d", ContainerPath: kubeletConfigDir}},
				}),
			),
		),
	))
}

// TestConfigDir checks the data behind CIS Kubernetes 4.1.11: the node
// configuration reports the kubelet drop-in directory and each .conf file
// the kubelet reads from it, with the ownership and permissions stat(1)
// sees.
func (s *kubeletConfigDirTestSuite) TestConfigDir() {
	worker := s.workerNode()
	pod := nodeAgentPod(s.T(), s.Env(), worker)
	s.writeDropins(worker, pod)

	typ, out := loadNodeConfig(s.T(), s.Env(), pod)
	assert.Equal(s.T(), "kubernetes_worker_node", typ)

	var node nodeConfig
	require.NoError(s.T(), json.Unmarshal([]byte(out), &node), out)
	require.NotNil(s.T(), node.Components.Kubelet, "kubelet missing from the node configuration")
	dir := node.Components.Kubelet.ConfigDir
	require.NotNil(s.T(), dir, "config-dir missing from the kubelet configuration")
	assertFileMeta(s.T(), statFile(s.T(), s.Env(), pod, kubeletConfigDir), &dir.fileMeta)

	// The kubelet walks the tree in lexical order and reads the files named
	// *.conf, README left alone.
	want := []string{kubeletConfigDir + "/10-e2e.conf", kubeletConfigDir + "/sub/20-e2e.conf"}
	require.Len(s.T(), dir.Files, len(want))
	for i, name := range want {
		assertFileMeta(s.T(), statFile(s.T(), s.Env(), pod, name), dir.Files[i])
	}
}

// workerNode returns the name of the kind worker.
func (s *kubeletConfigDirTestSuite) workerNode() string {
	nodes, err := s.Env().KubernetesCluster.Client().CoreV1().Nodes().List(context.Background(), metav1.ListOptions{
		LabelSelector: "!node-role.kubernetes.io/control-plane",
	})
	require.NoError(s.T(), err)
	require.Len(s.T(), nodes.Items, 1)
	return nodes.Items[0].Name
}

// writeDropins writes two drop-ins and a README into the kubelet drop-in
// directory of node, from a pod running the image of the agent pod agentPod,
// which the node holds already.
func (s *kubeletConfigDirTestSuite) writeDropins(node, agentPod string) {
	pods := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog")
	agent, err := pods.Get(context.Background(), agentPod, metav1.GetOptions{})
	require.NoError(s.T(), err)
	var image string
	for _, c := range agent.Spec.Containers {
		if c.Name == "security-agent" {
			image = c.Image
		}
	}
	require.NotEmpty(s.T(), image, "no security-agent container in pod %s", agentPod)

	const script = `set -e
conf='apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration'
echo "$conf" >/dropins/10-e2e.conf
mkdir -p /dropins/sub
echo "$conf" >/dropins/sub/20-e2e.conf
echo 'files without the .conf suffix are left alone' >/dropins/README
chmod 0644 /dropins/10-e2e.conf
chmod 0600 /dropins/sub/20-e2e.conf
`
	dirType := corev1.HostPathDirectory
	root := int64(0)
	writer := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "write-kubelet-dropins"},
		Spec: corev1.PodSpec{
			NodeName:      node,
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:            "write",
				Image:           image,
				Command:         []string{"sh", "-c", script},
				SecurityContext: &corev1.SecurityContext{RunAsUser: &root},
				VolumeMounts:    []corev1.VolumeMount{{Name: "dropins", MountPath: "/dropins"}},
			}},
			Volumes: []corev1.Volume{{
				Name: "dropins",
				VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{
					Path: kubeletConfigDir,
					Type: &dirType,
				}},
			}},
		},
	}
	_, err = pods.Create(context.Background(), writer, metav1.CreateOptions{})
	require.NoError(s.T(), err)
	var phase corev1.PodPhase
	require.Eventually(s.T(), func() bool {
		p, err := pods.Get(context.Background(), writer.Name, metav1.GetOptions{})
		if err != nil {
			return false
		}
		phase = p.Status.Phase
		return phase == corev1.PodSucceeded || phase == corev1.PodFailed
	}, 3*time.Minute, 5*time.Second, "pod %s did not complete", writer.Name)
	require.Equal(s.T(), corev1.PodSucceeded, phase, "pod %s", writer.Name)
}
