// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package external

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"

	"crypto/tls"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"path/filepath"

	"net/http/httptest"
	"os"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/clock"
	clocktesting "k8s.io/utils/clock/testing"

	kubeAutoscaling "github.com/DataDog/agent-payload/v5/autoscaling/kubernetes"
	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
	autoscalingv2 "k8s.io/api/autoscaling/v2"

	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestRecommenderClient_GetReplicaRecommendation(t *testing.T) {
	tests := []struct {
		name            string
		dpa             model.FakePodAutoscalerInternal
		expectedRequest *kubeAutoscaling.WorkloadRecommendationRequest
		serverResponse  *kubeAutoscaling.WorkloadRecommendationReply
		expectedError   string
	}{
		{
			name: "successful recommendation with CPU objective and watermarks",
			dpa: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "test-dpa",
				Spec: &datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "test-deployment",
						APIVersion: "apps/v1",
					},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
							PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
								Name: corev1.ResourceCPU,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Utilization: pointer.Ptr[int32](80),
								},
							},
						},
					},
					Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
						MinReplicas: pointer.Ptr[int32](2),
						MaxReplicas: pointer.Ptr[int32](4),
					},
				},
				CurrentReplicas: pointer.Ptr[int32](3),
				CustomRecommenderConfiguration: &model.RecommenderConfiguration{
					Endpoint: "",
					Settings: map[string]interface{}{
						"custom_setting": "value",
					},
				},
				ScalingValues: model.ScalingValues{
					Horizontal: &model.HorizontalScalingValues{
						Replicas: 3,
					},
				},
			},
			expectedRequest: &kubeAutoscaling.WorkloadRecommendationRequest{
				State: &kubeAutoscaling.WorkloadState{
					CurrentReplicas: pointer.Ptr[int32](3),
					ReadyReplicas:   pointer.Ptr[int32](1),
					DesiredReplicas: 3,
				},
				Targets: []*kubeAutoscaling.WorkloadRecommendationTarget{
					{
						Type:        "cpu",
						TargetValue: 0.80,
					},
				},
				Constraints: &kubeAutoscaling.WorkloadRecommendationConstraints{
					MinReplicas: 2,
					MaxReplicas: 4,
				},
				Settings: map[string]*structpb.Value{
					"custom_setting": structpb.NewStringValue("value"),
				},
			},
			serverResponse: &kubeAutoscaling.WorkloadRecommendationReply{
				TargetReplicas: 3,
			},
		},
		{
			name: "successful recommendation with container resource objective",
			dpa: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "test-dpa",
				Spec: &datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "test-deployment",
						APIVersion: "apps/v1",
					},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
							ContainerResource: &datadoghqcommon.DatadogPodAutoscalerContainerResourceObjective{
								Name: corev1.ResourceMemory,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Utilization: pointer.Ptr[int32](75),
								},
							},
						},
					},
				},
				CustomRecommenderConfiguration: &model.RecommenderConfiguration{
					Endpoint: "",
				},
			},
			expectedRequest: &kubeAutoscaling.WorkloadRecommendationRequest{
				Targets: []*kubeAutoscaling.WorkloadRecommendationTarget{
					{
						Type:        "memory",
						TargetValue: 0.75,
					},
				},
			},
			serverResponse: &kubeAutoscaling.WorkloadRecommendationReply{
				TargetReplicas: 5,
			},
		},
		{
			name: "successful recommendation with multiple objectives",
			dpa: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "test-dpa",
				Spec: &datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "test-deployment",
						APIVersion: "apps/v1",
					},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
							PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
								Name: corev1.ResourceCPU,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Utilization: pointer.Ptr[int32](80),
								},
							},
						},
						{
							Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
							ContainerResource: &datadoghqcommon.DatadogPodAutoscalerContainerResourceObjective{
								Name: corev1.ResourceMemory,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Utilization: pointer.Ptr[int32](75),
								},
							},
						},
					},
				},
				CustomRecommenderConfiguration: &model.RecommenderConfiguration{
					Endpoint: "",
				},
			},
			expectedRequest: &kubeAutoscaling.WorkloadRecommendationRequest{
				Targets: []*kubeAutoscaling.WorkloadRecommendationTarget{
					{
						Type:        "cpu",
						TargetValue: 0.80,
					},
					{
						Type:        "memory",
						TargetValue: 0.75,
					},
				},
			},
			serverResponse: &kubeAutoscaling.WorkloadRecommendationReply{
				TargetReplicas: 5,
			},
		},
		{
			name: "missing recommender config",
			dpa: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "test-dpa",
				Spec: &datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "test-deployment",
						APIVersion: "apps/v1",
					},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
							PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
								Name: corev1.ResourceCPU,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Utilization: pointer.Ptr[int32](80),
								},
							},
						},
					},
					Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
						MinReplicas: pointer.Ptr[int32](2),
						MaxReplicas: pointer.Ptr[int32](4),
					},
				},
			},
			expectedError: "external recommender spec is required",
		},
		{
			name: "invalid URL",
			dpa: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "test-dpa",
				Spec: &datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "test-deployment",
						APIVersion: "apps/v1",
					},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
							PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
								Name: corev1.ResourceCPU,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Utilization: pointer.Ptr[int32](80),
								},
							},
						},
					},
					Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
						MinReplicas: pointer.Ptr[int32](2),
						MaxReplicas: pointer.Ptr[int32](4),
					},
				},
				CustomRecommenderConfiguration: &model.RecommenderConfiguration{
					Endpoint: "http://in%val%%d",
				},
			},
			expectedError: "error parsing url: parse \"http://in%val%%d\": invalid URL escape \"%va\"",
		},
		{
			name: "invalid URL scheme",
			dpa: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "test-dpa",
				Spec: &datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "test-deployment",
						APIVersion: "apps/v1",
					},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
							PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
								Name: corev1.ResourceCPU,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Utilization: pointer.Ptr[int32](80),
								},
							},
						},
					},
					Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
						MinReplicas: pointer.Ptr[int32](2),
						MaxReplicas: pointer.Ptr[int32](4),
					},
				},
				CustomRecommenderConfiguration: &model.RecommenderConfiguration{
					Endpoint: "ftp://invalid-scheme",
				},
			},
			expectedError: "only http and https schemes are supported",
		},
		{
			name: "http call returns unexpected response code",
			dpa: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "test-dpa",
				Spec: &datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "test-deployment",
						APIVersion: "apps/v1",
					},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
							PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
								Name: corev1.ResourceCPU,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Utilization: pointer.Ptr[int32](80),
								},
							},
						},
					},
				},
				CustomRecommenderConfiguration: &model.RecommenderConfiguration{
					Endpoint: "",
					Settings: map[string]interface{}{
						"custom_setting": "value",
					},
				},
			},
			serverResponse: &kubeAutoscaling.WorkloadRecommendationReply{
				Error: &kubeAutoscaling.Error{
					Code:    pointer.Ptr[int32](404),
					Message: "Not Found",
				},
			},
			expectedError: "unexpected response code: 404",
		},
		{
			name: "recommender returns error",
			dpa: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "test-dpa",
				Spec: &datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "test-deployment",
						APIVersion: "apps/v1",
					},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
							PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
								Name: corev1.ResourceCPU,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Utilization: pointer.Ptr[int32](80),
								},
							},
						},
					},
				},
				CustomRecommenderConfiguration: &model.RecommenderConfiguration{
					Endpoint: "",
					Settings: map[string]interface{}{
						"custom_setting": "value",
					},
				},
			},
			expectedError: "error from recommender: 200 Some random error",
			serverResponse: &kubeAutoscaling.WorkloadRecommendationReply{
				Error: &kubeAutoscaling.Error{
					Code:    pointer.Ptr[int32](200),
					Message: "Some random error",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClock := clocktesting.NewFakeClock(time.Now())
			if tt.serverResponse != nil && tt.serverResponse.Timestamp == nil && tt.serverResponse.Error == nil {
				tt.serverResponse.Timestamp = timestamppb.New(fakeClock.Now())
			}

			server := startHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and headers
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.Equal(t, "datadog-cluster-agent", r.Header.Get("User-Agent"))

				// If we expect a specific request, verify it
				if tt.expectedRequest != nil {
					var actualRequest kubeAutoscaling.WorkloadRecommendationRequest
					body, err := io.ReadAll(r.Body)
					assert.NoError(t, err)
					err = protojson.Unmarshal(body, &actualRequest)
					assert.NoError(t, err)

					// Compare relevant fields
					if tt.expectedRequest.State != nil {
						assert.Equal(t, tt.expectedRequest.State.CurrentReplicas, actualRequest.State.CurrentReplicas)
						assert.Equal(t, tt.expectedRequest.State.DesiredReplicas, actualRequest.State.DesiredReplicas)
						assert.Equal(t, tt.expectedRequest.State.ReadyReplicas, actualRequest.State.ReadyReplicas)
					}
					if tt.expectedRequest.Targets != nil {
						assert.Equal(t, tt.expectedRequest.Targets[0].Type, actualRequest.Targets[0].Type)
						assert.Equal(t, tt.expectedRequest.Targets[0].LowerBound, actualRequest.Targets[0].LowerBound, 0.01)
						assert.Equal(t, tt.expectedRequest.Targets[0].UpperBound, actualRequest.Targets[0].UpperBound, 0.01)
					}
					if tt.expectedRequest.Constraints != nil {
						assert.Equal(t, tt.expectedRequest.Constraints.MinReplicas, actualRequest.Constraints.MinReplicas)
						assert.Equal(t, tt.expectedRequest.Constraints.MaxReplicas, actualRequest.Constraints.MaxReplicas)
					}
					if tt.expectedRequest.Settings != nil {
						// Compare settings values individually
						for k, expectedVal := range tt.expectedRequest.Settings {
							actualVal, ok := actualRequest.Settings[k]
							assert.True(t, ok, "Missing expected setting %s", k)
							assert.Equal(t, expectedVal.GetStringValue(), actualVal.GetStringValue(), "Setting %s value mismatch", k)
						}
					}
				}

				payload, err := protojson.Marshal(tt.serverResponse)
				if err != nil {
					t.Errorf("Failed to marshal response: %v", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				if tt.serverResponse.Error != nil {
					w.WriteHeader(int(*tt.serverResponse.Error.Code))
				} else {
					w.WriteHeader(http.StatusOK)
				}
				w.Write(payload)
			}))

			// The endpoint is only set for test cases that expect an error
			if tt.dpa.CustomRecommenderConfiguration != nil && tt.dpa.CustomRecommenderConfiguration.Endpoint == "" {
				tt.dpa.CustomRecommenderConfiguration.Endpoint = server.URL
			}

			pw := workload.NewPodWatcher(nil, nil)
			pw.HandleEvent(newFakeWLMPodEvent(tt.dpa.Namespace, tt.dpa.Spec.TargetRef.Name, "pod1", []string{"container-name1"}))

			client, err := newRecommenderClient(context.Background(), fakeClock, pw, nil)
			require.NoError(t, err)
			client.client = server.Client()

			result, err := client.GetReplicaRecommendation(context.Background(), "test-cluster", tt.dpa.Build())

			if tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
				assert.Nil(t, result)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.serverResponse.TargetReplicas, result.Replicas)
			assert.Equal(t, tt.serverResponse.Timestamp.AsTime(), result.Timestamp)
			assert.Equal(t, datadoghqcommon.DatadogPodAutoscalerExternalValueSource, result.Source)
		})
	}
}

func TestRecommenderClientTLSClientCertificateReload(t *testing.T) {
	fakeClock := clocktesting.NewFakeClock(time.Now())

	caCert, caKey, caPEM := generateTestCA(t, fakeClock)
	serverCertPEM, serverKeyPEM, _ := generateSignedCertificate(
		t,
		fakeClock,
		caCert,
		caKey,
		"server",
		[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		[]string{"localhost"},
		[]net.IP{net.ParseIP("127.0.0.1")},
		fakeClock.Now().Add(6*time.Hour),
	)

	serverTLSCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	require.NoError(t, err)
	serverTLSCert.Leaf, err = x509.ParseCertificate(serverTLSCert.Certificate[0])
	require.NoError(t, err)

	clientCert1PEM, clientKey1PEM, _ := generateSignedCertificate(
		t,
		fakeClock,
		caCert,
		caKey,
		"client-1",
		[]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		nil,
		nil,
		fakeClock.Now().Add(3*time.Hour),
	)

	tempDir := t.TempDir()
	caPath := filepath.Join(tempDir, "ca.pem")
	clientCertPath := filepath.Join(tempDir, "client.pem")
	clientKeyPath := filepath.Join(tempDir, "client.key")

	writePEM(t, caPath, caPEM)
	writePEM(t, clientCertPath, clientCert1PEM)
	writePEM(t, clientKeyPath, clientKey1PEM)

	responsePayload, err := protojson.Marshal(&kubeAutoscaling.WorkloadRecommendationReply{
		TargetReplicas: 3,
		Timestamp:      timestamppb.New(fakeClock.Now()),
	})
	require.NoError(t, err)

	clientCN := make(chan string, 2)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			clientCN <- r.TLS.PeerCertificates[0].Subject.CommonName
		} else {
			clientCN <- ""
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(responsePayload)
	})

	clientCAPool := x509.NewCertPool()
	require.True(t, clientCAPool.AppendCertsFromPEM(caPEM))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{serverTLSCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAPool,
		MinVersion:   tls.VersionTLS12,
	}
	server := &httptest.Server{
		Listener: listener,
		TLS:      serverTLS,
		Config:   &http.Server{Handler: handler},
	}
	server.StartTLS()
	t.Cleanup(server.Close)

	dpa := model.FakePodAutoscalerInternal{
		Namespace: "default",
		Name:      "tls-dpa",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Name:       "test-deployment",
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			},
		},
		CustomRecommenderConfiguration: &model.RecommenderConfiguration{
			Endpoint: server.URL,
		},
	}

	pw := workload.NewPodWatcher(nil, nil)
	pw.HandleEvent(newFakeWLMPodEvent(dpa.Namespace, dpa.Spec.TargetRef.Name, "pod1", []string{"container"}))

	client, err := newRecommenderClient(context.Background(), fakeClock, pw, &TLSFilesConfig{
		CAFile:   caPath,
		CertFile: clientCertPath,
		KeyFile:  clientKeyPath,
	})
	require.NoError(t, err)

	result, err := client.GetReplicaRecommendation(context.Background(), "test-cluster", dpa.Build())
	require.NoError(t, err)
	require.NotNil(t, result)

	select {
	case cn := <-clientCN:
		assert.Equal(t, "client-1", cn)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for client certificate")
	}
}

func generateTestCA(t *testing.T, clk clock.Clock) (*x509.Certificate, crypto.Signer, []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          randomSerial(t),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             clk.Now().Add(-time.Hour),
		NotAfter:              clk.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return cert, key, pemBytes
}

func generateSignedCertificate(t *testing.T, clk clock.Clock, caCert *x509.Certificate, caKey crypto.Signer, commonName string, extKeyUsage []x509.ExtKeyUsage, dnsNames []string, ips []net.IP, notAfter time.Time) ([]byte, []byte, *x509.Certificate) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          randomSerial(t),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             clk.Now().Add(-time.Hour),
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           extKeyUsage,
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           ips,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes})
	return certPEM, keyPEM, cert
}

func writePEM(t *testing.T, path string, data []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func randomSerial(t *testing.T) *big.Int {
	t.Helper()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	require.NoError(t, err)
	return serial
}

func startHTTPServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	if handler == nil {
		handler = http.DefaultServeMux
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: handler},
	}

	server.Start()
	t.Cleanup(server.Close)
	return server
}
