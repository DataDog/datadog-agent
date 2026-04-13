// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This is to define a mock registry for the gradual rollout E2E tests.
// This is to avoid actually making calls to the real registry during the tests.
// It should mock any kind of calls that the image resolver would make to the registry.

package ssigradualrollout

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
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
)

// mockRegistryScript is a Python HTTPS server that simulates a container registry.
// It uses self-signed TLS certs provided via TLS_CERT and TLS_KEY environment variables.
// - HEAD /v2/*/manifests/{N}-gr{N} (bucket-tagged): returns 200 + Docker-Content-Digest header
// - HEAD /v2/*/manifests/* (non-bucket-tagged, e.g. canonical "1.4.3"): returns 404
// - GET /healthz: returns 200
const mockRegistryScript = `
import http.server
import ssl
import re
import os

BUCKET_TAG_RE = re.compile(r'^/v2/[^/]+/manifests/\d+-gr\d+$')
FAKE_DIGEST = 'sha256:' + 'a' * 64

class RegistryHandler(http.server.BaseHTTPRequestHandler):
    def do_HEAD(self):
        if BUCKET_TAG_RE.match(self.path):
            self.send_response(200)
            self.send_header('Docker-Content-Digest', FAKE_DIGEST)
            self.end_headers()
        else:
            self.send_response(404)
            self.end_headers()

    def do_GET(self):
        if self.path == '/healthz':
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b'ok')
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, format, *args):
        pass

ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
ctx.load_cert_chain(os.environ['TLS_CERT'], os.environ['TLS_KEY'])

server = http.server.HTTPServer(('', 5000), RegistryHandler)
server.socket = ctx.wrap_socket(server.socket, server_side=True)
print('Mock registry listening on :5000', flush=True)
server.serve_forever()
`

// mockRegistryAddress returns the in-cluster address of the mock registry service.
func mockRegistryAddress() string {
	return fmt.Sprintf("mock-registry.mock-registry.svc.cluster.local:%d", mockRegistryPort)
}

// Package-level sync.Once for cert generation so certs are stable across UpdateEnv calls.
// Pulumi won't update Secrets unnecessarily if the data hasn't changed.
var (
	registryCertsOnce sync.Once
	cachedCACert      []byte
	cachedServerCert  []byte
	cachedServerKey   []byte
	cachedCertErr     error
)

// getCerts returns the cached CA cert, server cert, and server key PEM bytes,
// generating them on first call.
func getCerts() (caCertPEM, serverCertPEM, serverKeyPEM []byte, err error) {
	registryCertsOnce.Do(func() {
		cachedCACert, cachedServerCert, cachedServerKey, cachedCertErr = generateRegistryCerts()
	})
	return cachedCACert, cachedServerCert, cachedServerKey, cachedCertErr
}

// generateRegistryCerts generates a self-signed CA and a server certificate for the mock registry.
// Uses P-256 ECDSA keys. The server cert has DNSNames covering the in-cluster service FQDNs.
// Certs are valid for 24 hours.
func generateRegistryCerts() (caCertPEM, serverCertPEM, serverKeyPEM []byte, err error) {
	// Generate CA key.
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate CA key: %w", err)
	}

	// Create CA certificate template.
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

	// Self-sign the CA certificate.
	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	// Generate server key.
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate server key: %w", err)
	}

	// Create server certificate template.
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

	// Sign the server certificate with the CA.
	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create server certificate: %w", err)
	}

	// Encode CA cert to PEM.
	caCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})

	// Encode server cert to PEM.
	serverCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})

	// Encode server key to PEM.
	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to marshal server key: %w", err)
	}
	serverKeyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER})

	return caCertPEM, serverCertPEM, serverKeyPEM, nil
}

// deployMockRegistry deploys the mock container registry into the cluster.
// It creates:
//   - A "mock-registry" namespace for the registry pod and its secrets/configmap
//   - A "datadog" namespace (SSA handles conflict if the Helm chart also creates it)
//   - A CA Secret in the "datadog" namespace, mounted into the cluster-agent pod
//   - A TLS Secret in the "mock-registry" namespace for the Python server
//   - A ConfigMap in the "mock-registry" namespace containing server.py
//   - A Deployment running the Python HTTPS server
//   - A Service exposing port 5000
func deployMockRegistry(e config.Env, kubeProvider *kubernetes.Provider) error {
	caCertPEM, serverCertPEM, serverKeyPEM, err := getCerts()
	if err != nil {
		return fmt.Errorf("failed to get mock registry certs: %w", err)
	}

	baseOpts := []pulumi.ResourceOption{
		pulumi.Provider(kubeProvider),
	}

	// Create the mock-registry namespace.
	mockRegistryNs, err := corev1.NewNamespace(e.Ctx(), e.CommonNamer().ResourceName("mock-registry-ns"), &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(mockRegistryNamespace),
		},
	}, baseOpts...)
	if err != nil {
		return fmt.Errorf("failed to create mock-registry namespace: %w", err)
	}

	// The datadog namespace is also created by the Helm chart. Pulumi's server-side apply
	// (SDK v4 default) handles the conflict gracefully.
	datadogNs, err := corev1.NewNamespace(e.Ctx(), e.CommonNamer().ResourceName("datadog-ns"), &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String("datadog"),
		},
	}, baseOpts...)
	if err != nil {
		return fmt.Errorf("failed to create datadog namespace: %w", err)
	}

	// Mounted into the cluster-agent pod at /etc/ssl/certs/mock-registry-ca.crt (see base.yaml).
	// Go's x509.SystemCertPool() on Debian/Ubuntu scans /etc/ssl/certs/ and trusts all .crt files,
	// allowing the cluster-agent to verify the mock registry's TLS certificate.
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

	// TLS Secret for the mock registry server in the mock-registry namespace.
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

	// ConfigMap containing the Python server script.
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

	// Deployment running the Python HTTPS mock registry server.
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
							Image:   pulumi.String("python:3-slim"),
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

	// Service exposing port 5000 for the mock registry.
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
