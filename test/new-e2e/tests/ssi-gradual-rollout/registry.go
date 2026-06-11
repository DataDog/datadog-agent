// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ssigradualrollout

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
	"strconv"
	"sync"
	"time"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
)

const (
	mockRegistryNamespace = "mock-registry"
	mockRegistryName      = "mock-registry"
	mockRegistryPort      = 5000
	mockRegistryCaSecret  = "mock-registry-ca"

	// fakeRegistryDigest is the digest the mock registry returns for every bucket-tagged
	// manifest request. Tests assert the cluster-agent injects exactly this value, which
	// proves end-to-end that our mock served the digest (vs. the resolver finding one
	// elsewhere or fabricating a digest-shaped string). Kept in sync with server.py via
	// the FAKE_DIGEST env var.
	fakeRegistryDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

//go:embed testdata/server.py
var mockRegistryScript string

// Cached at package scope so cert bytes are stable across UpdateEnv calls — without
// this, every reprovision would diff the Secrets and trigger pod restarts.
var (
	registryCertsOnce sync.Once
	cachedCACert      []byte
	cachedServerCert  []byte
	cachedServerKey   []byte
	cachedCertErr     error
)

func getCerts() (caCertPEM, serverCertPEM, serverKeyPEM []byte, err error) {
	registryCertsOnce.Do(func() {
		cachedCACert, cachedServerCert, cachedServerKey, cachedCertErr = generateRegistryCerts()
	})
	return cachedCACert, cachedServerCert, cachedServerKey, cachedCertErr
}

func generateRegistryCerts() (caCertPEM, serverCertPEM, serverKeyPEM []byte, err error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate CA key: %w", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "mock-registry-ca",
			Organization: []string{"Datadog E2E Tests"},
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate server key: %w", err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName:   "mock-registry",
			Organization: []string{"Datadog E2E Tests"},
		},
		DNSNames: []string{
			"mock-registry",
			"mock-registry.mock-registry.svc",
			"mock-registry.mock-registry.svc.cluster",
			"mock-registry.mock-registry.svc.cluster.local",
		},
		NotBefore: time.Now().Add(-time.Minute),
		NotAfter:  time.Now().Add(24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}

	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create server certificate: %w", err)
	}

	caCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})
	serverCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})

	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to marshal server key: %w", err)
	}
	serverKeyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER})

	return caCertPEM, serverCertPEM, serverKeyPEM, nil
}

func deployMockRegistry(e config.Env, kubeProvider *kubernetes.Provider) error {
	caCertPEM, serverCertPEM, serverKeyPEM, err := getCerts()
	if err != nil {
		return fmt.Errorf("failed to get mock registry certs: %w", err)
	}

	baseOpts := []pulumi.ResourceOption{
		pulumi.Provider(kubeProvider),
	}

	mockRegistryNs, err := corev1.NewNamespace(e.Ctx(), e.CommonNamer().ResourceName("mock-registry-ns"), &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(mockRegistryNamespace),
		},
	}, baseOpts...)
	if err != nil {
		return fmt.Errorf("failed to create mock-registry namespace: %w", err)
	}

	// The Helm chart also creates the datadog namespace; Pulumi's server-side apply
	// (SDK v4 default) reconciles the shared ownership without conflict.
	datadogNs, err := corev1.NewNamespace(e.Ctx(), e.CommonNamer().ResourceName("datadog-ns"), &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String("datadog"),
		},
	}, baseOpts...)
	if err != nil {
		return fmt.Errorf("failed to create datadog namespace: %w", err)
	}

	// Mounted into the cluster-agent at /etc/ssl/certs/mock-registry-ca.crt (see
	// default_opt_in.yaml). Go's x509.SystemCertPool() on the cluster-agent's Debian
	// base image scans that dir and trusts every .crt, so no Go-side cert plumbing
	// is needed for the resolver to verify the mock's TLS cert.
	_, err = corev1.NewSecret(e.Ctx(), e.CommonNamer().ResourceName("mock-registry-ca-secret"), &corev1.SecretArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(mockRegistryCaSecret),
			Namespace: pulumi.String("datadog"),
		},
		StringData: pulumi.StringMap{
			"ca.crt": pulumi.String(string(caCertPEM)),
		},
	}, append(baseOpts, pulumi.DependsOn([]pulumi.Resource{datadogNs}))...)
	if err != nil {
		return fmt.Errorf("failed to create mock-registry CA secret: %w", err)
	}

	tlsSecret, err := corev1.NewSecret(e.Ctx(), e.CommonNamer().ResourceName("mock-registry-tls-secret"), &corev1.SecretArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("mock-registry-tls"),
			Namespace: pulumi.String(mockRegistryNamespace),
		},
		StringData: pulumi.StringMap{
			"tls.crt": pulumi.String(string(serverCertPEM)),
			"tls.key": pulumi.String(string(serverKeyPEM)),
		},
	}, append(baseOpts, pulumi.DependsOn([]pulumi.Resource{mockRegistryNs}))...)
	if err != nil {
		return fmt.Errorf("failed to create mock-registry TLS secret: %w", err)
	}

	scriptCM, err := corev1.NewConfigMap(e.Ctx(), e.CommonNamer().ResourceName("mock-registry-script-cm"), &corev1.ConfigMapArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("mock-registry-script"),
			Namespace: pulumi.String(mockRegistryNamespace),
		},
		Data: pulumi.StringMap{
			"server.py": pulumi.String(mockRegistryScript),
		},
	}, append(baseOpts, pulumi.DependsOn([]pulumi.Resource{mockRegistryNs}))...)
	if err != nil {
		return fmt.Errorf("failed to create mock-registry script configmap: %w", err)
	}

	_, err = appsv1.NewDeployment(e.Ctx(), e.CommonNamer().ResourceName("mock-registry-deployment"), &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(mockRegistryName),
			Namespace: pulumi.String(mockRegistryNamespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String(mockRegistryName),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String(mockRegistryName),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app": pulumi.String(mockRegistryName),
					},
				},
				Spec: &corev1.PodSpecArgs{
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:    pulumi.String(mockRegistryName),
							Image:   pulumi.String("public.ecr.aws/docker/library/python:3-slim"),
							Command: pulumi.StringArray{pulumi.String("python"), pulumi.String("/scripts/server.py")},
							Env: corev1.EnvVarArray{
								&corev1.EnvVarArgs{
									Name:  pulumi.String("TLS_CERT"),
									Value: pulumi.String("/certs/tls.crt"),
								},
								&corev1.EnvVarArgs{
									Name:  pulumi.String("TLS_KEY"),
									Value: pulumi.String("/certs/tls.key"),
								},
								&corev1.EnvVarArgs{
									Name:  pulumi.String("PORT"),
									Value: pulumi.String(strconv.Itoa(mockRegistryPort)),
								},
								&corev1.EnvVarArgs{
									Name:  pulumi.String("FAKE_DIGEST"),
									Value: pulumi.String(fakeRegistryDigest),
								},
							},
							VolumeMounts: corev1.VolumeMountArray{
								&corev1.VolumeMountArgs{
									Name:      pulumi.String("scripts"),
									MountPath: pulumi.String("/scripts"),
								},
								&corev1.VolumeMountArgs{
									Name:      pulumi.String("certs"),
									MountPath: pulumi.String("/certs"),
									ReadOnly:  pulumi.Bool(true),
								},
							},
							ReadinessProbe: &corev1.ProbeArgs{
								TcpSocket: &corev1.TCPSocketActionArgs{
									Port: pulumi.Int(mockRegistryPort),
								},
								InitialDelaySeconds: pulumi.Int(5),
								PeriodSeconds:       pulumi.Int(10),
							},
						},
					},
					Volumes: corev1.VolumeArray{
						&corev1.VolumeArgs{
							Name: pulumi.String("scripts"),
							ConfigMap: &corev1.ConfigMapVolumeSourceArgs{
								Name: pulumi.String("mock-registry-script"),
							},
						},
						&corev1.VolumeArgs{
							Name: pulumi.String("certs"),
							Secret: &corev1.SecretVolumeSourceArgs{
								SecretName: pulumi.String("mock-registry-tls"),
							},
						},
					},
				},
			},
		},
	}, append(baseOpts, pulumi.DependsOn([]pulumi.Resource{tlsSecret, scriptCM}))...)
	if err != nil {
		return fmt.Errorf("failed to create mock-registry deployment: %w", err)
	}

	_, err = corev1.NewService(e.Ctx(), e.CommonNamer().ResourceName("mock-registry-service"), &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(mockRegistryName),
			Namespace: pulumi.String(mockRegistryNamespace),
		},
		Spec: &corev1.ServiceSpecArgs{
			Selector: pulumi.StringMap{
				"app": pulumi.String(mockRegistryName),
			},
			Ports: corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Port:     pulumi.Int(mockRegistryPort),
					Protocol: pulumi.String("TCP"),
				},
			},
		},
	}, append(baseOpts, pulumi.DependsOn([]pulumi.Resource{mockRegistryNs}))...)
	if err != nil {
		return fmt.Errorf("failed to create mock-registry service: %w", err)
	}

	return nil
}
