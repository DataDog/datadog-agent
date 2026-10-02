// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenthealth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	_ "embed"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	kubecomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
)

const (
	remotePersistenceNamespace       = "remote-persistence"
	remotePersistenceBackendName     = "api"
	remotePersistenceBackendPort     = 8443
	remotePersistenceCASecret        = "remote-persistence-ca"
	remotePersistenceTLSSecret       = "remote-persistence-tls"
	remotePersistenceScriptConfigMap = "remote-persistence-backend"
	remotePersistencePythonImage     = "python:3.14.5@sha256:250e5c97be05e1eb2272fbdbd810dfd638f9012e1e6f65c99390ad3239943a08"
	staleNodeIssueID                 = "invalid-config:e2e-stale-node"
	staleClusterIssueID              = "invalid-config:e2e-stale-cluster"
)

//go:embed fixtures/remote_persistence_backend.py
var remotePersistenceBackendScript string

var (
	remotePersistenceCertsOnce sync.Once
	remotePersistenceCACert    []byte
	remotePersistenceCert      []byte
	remotePersistenceKey       []byte
	remotePersistenceCertErr   error
)

func getRemotePersistenceCerts() (caCert, serverCert, serverKey []byte, err error) {
	remotePersistenceCertsOnce.Do(func() {
		remotePersistenceCACert, remotePersistenceCert, remotePersistenceKey, remotePersistenceCertErr = generateRemotePersistenceCerts()
	})
	return remotePersistenceCACert, remotePersistenceCert, remotePersistenceKey, remotePersistenceCertErr
}

func remotePersistenceBackendImage(e config.Env) string {
	registry := strings.SplitN(e.ImagePullRegistry(), ",", 2)[0]
	if registry == "" {
		return remotePersistencePythonImage
	}
	return registry + "/dockerhub/library/" + remotePersistencePythonImage
}

func generateRemotePersistenceCerts() (caCertPEM, serverCertPEM, serverKeyPEM []byte, err error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("generate CA key: %w", err)
	}

	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "remote-persistence-e2e-ca",
			Organization: []string{"Datadog E2E Tests"},
		},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create CA certificate: %w", err)
	}
	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse CA certificate: %w", err)
	}

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("generate server key: %w", err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName:   "api.remote-persistence.svc.cluster.local",
			Organization: []string{"Datadog E2E Tests"},
		},
		DNSNames: []string{
			"api",
			"api.remote-persistence",
			"api.remote-persistence.svc",
			"api.remote-persistence.svc.cluster.local",
		},
		NotBefore:   now.Add(-time.Minute),
		NotAfter:    now.Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create server certificate: %w", err)
	}
	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("marshal server key: %w", err)
	}

	caCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})
	serverCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})
	serverKeyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER})
	return caCertPEM, serverCertPEM, serverKeyPEM, nil
}

func deployRemotePersistenceBackend(e config.Env, kubeProvider *kubernetes.Provider) (*kubecomp.Workload, error) {
	caCert, serverCert, serverKey, err := getRemotePersistenceCerts()
	if err != nil {
		return nil, err
	}

	workload := &kubecomp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", "remote-persistence-backend", workload); err != nil {
		return nil, err
	}
	baseOpts := []pulumi.ResourceOption{pulumi.Provider(kubeProvider), pulumi.Parent(workload)}

	backendNamespace, err := corev1.NewNamespace(e.Ctx(), e.CommonNamer().ResourceName("remote-persistence-namespace"), &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(remotePersistenceNamespace)},
	}, baseOpts...)
	if err != nil {
		return nil, fmt.Errorf("create remote persistence namespace: %w", err)
	}

	datadogNamespace, err := corev1.NewNamespacePatch(e.Ctx(), e.CommonNamer().ResourceName("remote-persistence-datadog-namespace"), &corev1.NamespacePatchArgs{
		Metadata: &metav1.ObjectMetaPatchArgs{Name: pulumi.String(clusterAgentNamespace)},
	}, baseOpts...)
	if err != nil {
		return nil, fmt.Errorf("patch Datadog namespace: %w", err)
	}

	_, err = corev1.NewSecret(e.Ctx(), e.CommonNamer().ResourceName("remote-persistence-ca"), &corev1.SecretArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(remotePersistenceCASecret),
			Namespace: pulumi.String(clusterAgentNamespace),
		},
		StringData: pulumi.StringMap{"ca.crt": pulumi.String(string(caCert))},
	}, append(baseOpts, pulumi.DependsOn([]pulumi.Resource{datadogNamespace}))...)
	if err != nil {
		return nil, fmt.Errorf("create remote persistence CA secret: %w", err)
	}

	tlsSecret, err := corev1.NewSecret(e.Ctx(), e.CommonNamer().ResourceName("remote-persistence-tls"), &corev1.SecretArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(remotePersistenceTLSSecret),
			Namespace: pulumi.String(remotePersistenceNamespace),
		},
		StringData: pulumi.StringMap{
			"tls.crt": pulumi.String(string(serverCert)),
			"tls.key": pulumi.String(string(serverKey)),
		},
	}, append(baseOpts, pulumi.DependsOn([]pulumi.Resource{backendNamespace}))...)
	if err != nil {
		return nil, fmt.Errorf("create remote persistence TLS secret: %w", err)
	}

	scriptConfig, err := corev1.NewConfigMap(e.Ctx(), e.CommonNamer().ResourceName("remote-persistence-script"), &corev1.ConfigMapArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(remotePersistenceScriptConfigMap),
			Namespace: pulumi.String(remotePersistenceNamespace),
		},
		Data: pulumi.StringMap{"server.py": pulumi.String(remotePersistenceBackendScript)},
	}, append(baseOpts, pulumi.DependsOn([]pulumi.Resource{backendNamespace}))...)
	if err != nil {
		return nil, fmt.Errorf("create remote persistence script ConfigMap: %w", err)
	}

	var imagePullSecrets corev1.LocalObjectReferenceArray
	if e.ImagePullRegistry() != "" {
		imagePullSecret, err := utils.NewImagePullSecret(
			e,
			remotePersistenceNamespace,
			append(baseOpts, pulumi.DependsOn([]pulumi.Resource{backendNamespace}))...,
		)
		if err != nil {
			return nil, fmt.Errorf("create remote persistence image pull secret: %w", err)
		}
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReferenceArgs{
			Name: imagePullSecret.Metadata.Name(),
		})
	}

	labels := pulumi.StringMap{"app": pulumi.String(remotePersistenceBackendName)}
	deployment, err := appsv1.NewDeployment(e.Ctx(), e.CommonNamer().ResourceName("remote-persistence-deployment"), &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(remotePersistenceBackendName),
			Namespace: pulumi.String(remotePersistenceNamespace),
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{MatchLabels: labels},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{Labels: labels},
				Spec: &corev1.PodSpecArgs{
					ImagePullSecrets: imagePullSecrets,
					Containers: corev1.ContainerArray{&corev1.ContainerArgs{
						Name:    pulumi.String(remotePersistenceBackendName),
						Image:   pulumi.String(remotePersistenceBackendImage(e)),
						Command: pulumi.StringArray{pulumi.String("python"), pulumi.String("/scripts/server.py")},
						Env: corev1.EnvVarArray{
							&corev1.EnvVarArgs{Name: pulumi.String("TLS_CERT"), Value: pulumi.String("/certs/tls.crt")},
							&corev1.EnvVarArgs{Name: pulumi.String("TLS_KEY"), Value: pulumi.String("/certs/tls.key")},
							&corev1.EnvVarArgs{Name: pulumi.String("STALE_NODE_ISSUE_ID"), Value: pulumi.String(staleNodeIssueID)},
							&corev1.EnvVarArgs{Name: pulumi.String("STALE_CLUSTER_ISSUE_ID"), Value: pulumi.String(staleClusterIssueID)},
						},
						Ports: corev1.ContainerPortArray{&corev1.ContainerPortArgs{ContainerPort: pulumi.Int(remotePersistenceBackendPort)}},
						ReadinessProbe: &corev1.ProbeArgs{
							HttpGet: &corev1.HTTPGetActionArgs{
								Path:   pulumi.String("/health"),
								Port:   pulumi.Int(remotePersistenceBackendPort),
								Scheme: pulumi.String("HTTPS"),
							},
							InitialDelaySeconds: pulumi.Int(2),
							PeriodSeconds:       pulumi.Int(2),
						},
						VolumeMounts: corev1.VolumeMountArray{
							&corev1.VolumeMountArgs{Name: pulumi.String("script"), MountPath: pulumi.String("/scripts"), ReadOnly: pulumi.Bool(true)},
							&corev1.VolumeMountArgs{Name: pulumi.String("tls"), MountPath: pulumi.String("/certs"), ReadOnly: pulumi.Bool(true)},
						},
					}},
					Volumes: corev1.VolumeArray{
						&corev1.VolumeArgs{
							Name: pulumi.String("script"),
							ConfigMap: &corev1.ConfigMapVolumeSourceArgs{
								Name: pulumi.String(remotePersistenceScriptConfigMap),
							},
						},
						&corev1.VolumeArgs{
							Name: pulumi.String("tls"),
							Secret: &corev1.SecretVolumeSourceArgs{
								SecretName: pulumi.String(remotePersistenceTLSSecret),
							},
						},
					},
				},
			},
		},
	}, append(baseOpts, pulumi.DependsOn([]pulumi.Resource{scriptConfig, tlsSecret}))...)
	if err != nil {
		return nil, fmt.Errorf("create remote persistence backend Deployment: %w", err)
	}

	_, err = corev1.NewService(e.Ctx(), e.CommonNamer().ResourceName("remote-persistence-service"), &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(remotePersistenceBackendName),
			Namespace: pulumi.String(remotePersistenceNamespace),
		},
		Spec: &corev1.ServiceSpecArgs{
			Selector: labels,
			Ports: corev1.ServicePortArray{&corev1.ServicePortArgs{
				Port:       pulumi.Int(443),
				TargetPort: pulumi.Int(remotePersistenceBackendPort),
			}},
		},
	}, append(baseOpts, pulumi.DependsOn([]pulumi.Resource{deployment}))...)
	if err != nil {
		return nil, fmt.Errorf("create remote persistence backend Service: %w", err)
	}

	return workload, nil
}
