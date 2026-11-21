// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kafka

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

func TestParseBootstrapServers(t *testing.T) {
	assert.Equal(t, []string{"broker-1:9093", "broker-2:9093"}, parseBootstrapServers(" broker-1:9093, ,broker-2:9093,"))
	assert.Empty(t, parseBootstrapServers(" , "))
}

func TestNewClientOptionsRequiresBootstrapServers(t *testing.T) {
	_, err := newClientOptions(map[string]string{bootstrapServersToken: " "})
	require.ErrorContains(t, err, "invalid bootstrapServers")
}

func TestGetSASLMechanism(t *testing.T) {
	tests := []struct {
		name      string
		tokens    map[string]string
		mechanism string
		wantErr   string
	}{
		{name: "no authentication", tokens: map[string]string{}},
		{name: "plain", tokens: map[string]string{saslMechanismToken: "PLAIN", usernameToken: "user", passwordToken: "secret"}, mechanism: "PLAIN"},
		{name: "scram sha 256", tokens: map[string]string{saslMechanismToken: "SCRAM-SHA-256", usernameToken: "user", passwordToken: "secret"}, mechanism: "SCRAM-SHA-256"},
		{name: "scram sha 512 is case insensitive", tokens: map[string]string{saslMechanismToken: " scram-sha-512 ", usernameToken: "user", passwordToken: "secret"}, mechanism: "SCRAM-SHA-512"},
		{name: "username without mechanism", tokens: map[string]string{usernameToken: "user", passwordToken: "secret"}, wantErr: "invalid saslMechanism"},
		{name: "missing username", tokens: map[string]string{saslMechanismToken: "PLAIN", passwordToken: "secret"}, wantErr: "invalid username"},
		{name: "missing password", tokens: map[string]string{saslMechanismToken: "PLAIN", usernameToken: "user"}, wantErr: "invalid password"},
		{name: "unsupported mechanism", tokens: map[string]string{saslMechanismToken: "GSSAPI", usernameToken: "user", passwordToken: "secret"}, wantErr: `invalid saslMechanism "GSSAPI"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mechanism, err := getSASLMechanism(tt.tokens)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.NotContains(t, err.Error(), "secret")
				return
			}
			require.NoError(t, err)
			if tt.mechanism == "" {
				assert.Nil(t, mechanism)
				return
			}
			require.NotNil(t, mechanism)
			assert.Equal(t, tt.mechanism, mechanism.Name())
		})
	}
}

func TestGetTLSConfig(t *testing.T) {
	certificate, key := newTestCertificate(t)
	_, otherKey := newTestCertificate(t)

	tests := []struct {
		name    string
		tokens  map[string]string
		check   func(t *testing.T, config *tls.Config)
		wantErr string
	}{
		{
			name:   "enabled by default",
			tokens: map[string]string{},
			check: func(t *testing.T, config *tls.Config) {
				require.NotNil(t, config)
				assert.Equal(t, uint16(tls.VersionTLS12), config.MinVersion)
				assert.False(t, config.InsecureSkipVerify)
				assert.Nil(t, config.RootCAs)
				assert.Empty(t, config.Certificates)
			},
		},
		{
			name:   "disabled",
			tokens: map[string]string{tlsToken: "false"},
			check: func(t *testing.T, config *tls.Config) {
				assert.Nil(t, config)
			},
		},
		{
			name:   "custom CA, client certificate and server name",
			tokens: map[string]string{tlsToken: "true", tlsCACertificateToken: certificate, tlsClientCertificateToken: certificate, tlsClientKeyToken: key, tlsServerNameToken: "kafka.internal"},
			check: func(t *testing.T, config *tls.Config) {
				require.NotNil(t, config)
				assert.NotNil(t, config.RootCAs)
				assert.Len(t, config.Certificates, 1)
				assert.Equal(t, "kafka.internal", config.ServerName)
			},
		},
		{name: "invalid boolean", tokens: map[string]string{tlsToken: "maybe"}, wantErr: `invalid tls "maybe"`},
		{name: "options while disabled", tokens: map[string]string{tlsToken: "false", tlsCACertificateToken: certificate}, wantErr: "TLS options are set but tls is false"},
		{name: "invalid CA", tokens: map[string]string{tlsCACertificateToken: "not a certificate"}, wantErr: "invalid tlsCaCertificate"},
		{name: "client certificate without key", tokens: map[string]string{tlsClientCertificateToken: certificate}, wantErr: "both tlsClientCertificate and tlsClientKey are required"},
		{name: "client key without certificate", tokens: map[string]string{tlsClientKeyToken: key}, wantErr: "both tlsClientCertificate and tlsClientKey are required"},
		{name: "mismatched client key", tokens: map[string]string{tlsClientCertificateToken: certificate, tlsClientKeyToken: otherKey}, wantErr: "invalid TLS client authentication"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := getTLSConfig(tt.tokens)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.NotContains(t, err.Error(), "PRIVATE KEY")
				return
			}
			require.NoError(t, err)
			tt.check(t, config)
		})
	}
}

func TestNewAdminClient(t *testing.T) {
	_, err := newAdminClient(nil)
	require.ErrorContains(t, err, "missing Kafka connection")

	_, err = newAdminClient(&privateconnection.PrivateCredentials{Type: privateconnection.TokenAuthType})
	require.ErrorContains(t, err, "invalid bootstrapServers")

	// Creating the client does not contact the brokers.
	client, err := newAdminClient(&privateconnection.PrivateCredentials{
		Type: privateconnection.TokenAuthType,
		Tokens: []privateconnection.PrivateCredentialsToken{
			{Name: bootstrapServersToken, Value: "127.0.0.1:1,127.0.0.1:2"},
			{Name: saslMechanismToken, Value: "SCRAM-SHA-512"},
			{Name: usernameToken, Value: "user"},
			{Name: passwordToken, Value: "secret"},
		},
	})
	require.NoError(t, err)
	client.Close()
}

func TestDescribeGroup(t *testing.T) {
	instanceID := "validator-0"
	consumerProtocolGroup := kadm.DescribedConsumerGroups{testGroup: {
		Group: testGroup, State: "Stable", Coordinator: kadm.BrokerDetail{NodeID: 2},
		Members: []kadm.ConsumerGroupMember{{
			MemberID: "member-1", ClientID: "validator", ClientHost: "/10.0.0.1",
			Assignment: kadm.TopicsSet{testTopic: {1: {}, 0: {}}},
		}},
	}}
	wantConsumerProtocolGroup := &ConsumerGroup{
		Group: testGroup, State: "Stable", GroupProtocol: consumerGroupProtocol, CoordinatorID: int32Ptr(2),
		Members: []GroupMember{{MemberID: "member-1", ClientID: "validator", ClientHost: "/10.0.0.1", Assignments: []TopicPartitions{{Topic: testTopic, Partitions: []int32{0, 1}}}}},
	}
	tests := []struct {
		name    string
		client  *fakeAdminClient
		want    *ConsumerGroup
		wantErr string
	}{
		{
			name: "classic group",
			client: &fakeAdminClient{groups: newClassicGroup("Stable", kadm.DescribedGroupMember{
				MemberID: "member-1", InstanceID: &instanceID, ClientID: "validator", ClientHost: "/10.0.0.1",
			})},
			want: &ConsumerGroup{
				Group: testGroup, State: "Stable", GroupProtocol: classicGroupProtocol, ProtocolType: "consumer", CoordinatorID: int32Ptr(1),
				Members: []GroupMember{{MemberID: "member-1", InstanceID: "validator-0", ClientID: "validator", ClientHost: "/10.0.0.1", Assignments: []TopicPartitions{}}},
			},
		},
		{
			name: "classic group on a broker without KIP-848",
			client: &fakeAdminClient{
				kip848Unsupported: true,
				groups:            newClassicGroup(groupStateEmpty),
				consumerGroupsErr: errors.New("the KIP-848 API must not be called"),
			},
			want: &ConsumerGroup{Group: testGroup, State: groupStateEmpty, GroupProtocol: classicGroupProtocol, ProtocolType: "consumer", CoordinatorID: int32Ptr(1), Members: []GroupMember{}},
		},
		{
			name: "consumer protocol group reported as missing by the classic API",
			client: &fakeAdminClient{
				groups:         kadm.DescribedGroups{testGroup: {Group: testGroup, State: groupStateDead, Err: kerr.GroupIDNotFound}},
				consumerGroups: consumerProtocolGroup,
			},
			want: wantConsumerProtocolGroup,
		},
		{
			name: "consumer protocol group reported without members by the classic API",
			client: &fakeAdminClient{
				groups:         newClassicGroup("Stable"),
				consumerGroups: consumerProtocolGroup,
			},
			want: wantConsumerProtocolGroup,
		},
		{
			name:   "unknown group",
			client: &fakeAdminClient{groups: kadm.DescribedGroups{testGroup: {Group: testGroup, State: groupStateDead, Err: kerr.GroupIDNotFound}}},
			want:   &ConsumerGroup{Group: testGroup, State: groupStateDead, GroupProtocol: classicGroupProtocol, Members: []GroupMember{}},
		},
		{
			name:   "unknown group on a broker that predates KIP-848 and KIP-1043",
			client: &fakeAdminClient{kip848Unsupported: true},
			want:   &ConsumerGroup{Group: testGroup, State: groupStateDead, GroupProtocol: classicGroupProtocol, CoordinatorID: int32Ptr(0), Members: []GroupMember{}},
		},
		{
			// The classic API would report a live KIP-848 group as missing.
			name: "KIP-848 request error",
			client: &fakeAdminClient{
				groups:            kadm.DescribedGroups{testGroup: {Group: testGroup, State: groupStateDead, Err: kerr.GroupIDNotFound}},
				consumerGroupsErr: errors.New("request timed out"),
			},
			wantErr: "request timed out",
		},
		{
			name: "KIP-848 group error",
			client: &fakeAdminClient{
				consumerGroups: kadm.DescribedConsumerGroups{testGroup: {Group: testGroup, Err: kerr.CoordinatorLoadInProgress}},
			},
			wantErr: "COORDINATOR_LOAD_IN_PROGRESS",
		},
		{
			name:    "broker API versions unavailable",
			client:  &fakeAdminClient{apiVersionsErr: errors.New("no broker reported its API versions")},
			wantErr: "no broker reported its API versions",
		},
		{
			name:    "describe error",
			client:  &fakeAdminClient{groups: kadm.DescribedGroups{testGroup: {Group: testGroup, Err: kerr.CoordinatorNotAvailable}}},
			wantErr: "COORDINATOR_NOT_AVAILABLE",
		},
		{
			name:    "request error",
			client:  &fakeAdminClient{groupsErr: errors.New("unable to dial")},
			wantErr: "unable to dial",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group, err := describeGroup(context.Background(), tt.client, testGroup)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, group)
		})
	}
}

func int32Ptr(v int32) *int32 {
	return &v
}

// newTestCertificate returns a self-signed certificate and its key as PEM.
func newTestCertificate(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "kafka-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	certificate, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	privateKey, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKey}))
}
