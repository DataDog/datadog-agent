// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kafka

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

// Connection token names. The runner resolves their values, usually from a
// credentials file on the runner host, so broker secrets stay on the runner.
const (
	bootstrapServersToken     = "bootstrapServers"
	saslMechanismToken        = "saslMechanism"
	usernameToken             = privateconnection.UsernameTokenName
	passwordToken             = privateconnection.PasswordTokenName
	tlsToken                  = "tls"
	tlsCACertificateToken     = "tlsCaCertificate"
	tlsClientCertificateToken = "tlsClientCertificate"
	tlsClientKeyToken         = "tlsClientKey"
	tlsServerNameToken        = "tlsServerName"
)

const (
	clientID = "datadog-private-action-runner"

	groupStateEmpty = "Empty"
	groupStateDead  = "Dead"

	classicGroupProtocol  = "classic"
	consumerGroupProtocol = "consumer"
)

// adminClient is the subset of *kadm.Client used by the actions.
type adminClient interface {
	BrokerMetadata(ctx context.Context) (kadm.Metadata, error)
	SupportsConsumerGroupDescribe(ctx context.Context) (bool, error)
	DescribeGroups(ctx context.Context, groups ...string) (kadm.DescribedGroups, error)
	DescribeConsumerGroups(ctx context.Context, groups ...string) (kadm.DescribedConsumerGroups, error)
	FetchOffsets(ctx context.Context, group string) (kadm.OffsetResponses, error)
	FetchOffsetsForTopics(ctx context.Context, group string, topics ...string) (kadm.OffsetResponses, error)
	ListStartOffsets(ctx context.Context, topics ...string) (kadm.ListedOffsets, error)
	ListEndOffsets(ctx context.Context, topics ...string) (kadm.ListedOffsets, error)
	CommitOffsets(ctx context.Context, group string, offsets kadm.Offsets) (kadm.OffsetResponses, error)
	Close()
}

type kadmClient struct {
	*kadm.Client
}

var _ adminClient = kadmClient{}

// SupportsConsumerGroupDescribe reports whether the brokers implement the
// KIP-848 ConsumerGroupDescribe API (Kafka 3.7+).
func (c kadmClient) SupportsConsumerGroupDescribe(ctx context.Context) (bool, error) {
	versions, err := c.ApiVersions(ctx)
	if err != nil {
		return false, err
	}
	var errs []error
	for _, broker := range versions {
		if broker.Err != nil {
			errs = append(errs, broker.Err)
			continue
		}
		if _, ok := broker.KeyMaxVersion(kmsg.ConsumerGroupDescribe.Int16()); ok {
			return true, nil
		}
	}
	if len(errs) > 0 && len(errs) == len(versions) {
		return false, fmt.Errorf("no broker reported its API versions: %w", errors.Join(errs...))
	}
	return false, nil
}

type adminClientFactory func(credential *privateconnection.PrivateCredentials) (adminClient, error)

// newAdminClient does not contact the brokers, so its errors are configuration errors.
func newAdminClient(credential *privateconnection.PrivateCredentials) (adminClient, error) {
	if credential == nil {
		return nil, errors.New("missing Kafka connection")
	}
	opts, err := newClientOptions(credential.AsTokenMap())
	if err != nil {
		return nil, err
	}
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("unable to create Kafka client: %w", err)
	}
	return kadmClient{kadm.NewClient(client)}, nil
}

func newClientOptions(credentialTokens map[string]string) ([]kgo.Opt, error) {
	seeds := parseBootstrapServers(credentialTokens[bootstrapServersToken])
	if len(seeds) == 0 {
		return nil, errors.New("invalid bootstrapServers: at least one broker address is required")
	}
	opts := []kgo.Opt{kgo.SeedBrokers(seeds...), kgo.ClientID(clientID)}

	mechanism, err := getSASLMechanism(credentialTokens)
	if err != nil {
		return nil, err
	}
	if mechanism != nil {
		opts = append(opts, kgo.SASL(mechanism))
	}

	tlsConfig, err := getTLSConfig(credentialTokens)
	if err != nil {
		return nil, err
	}
	if tlsConfig != nil {
		opts = append(opts, kgo.DialTLSConfig(tlsConfig))
	}
	return opts, nil
}

func parseBootstrapServers(value string) []string {
	var seeds []string
	for _, seed := range strings.Split(value, ",") {
		if seed = strings.TrimSpace(seed); seed != "" {
			seeds = append(seeds, seed)
		}
	}
	return seeds
}

func getSASLMechanism(credentialTokens map[string]string) (sasl.Mechanism, error) {
	mechanism := strings.ToUpper(strings.TrimSpace(credentialTokens[saslMechanismToken]))
	username := credentialTokens[usernameToken]
	password := credentialTokens[passwordToken]
	if mechanism == "" {
		// Refuse to silently connect without authentication.
		if username != "" || password != "" {
			return nil, errors.New("invalid saslMechanism: a SASL mechanism is required when a username or password is set")
		}
		return nil, nil
	}
	if username == "" {
		return nil, errors.New("invalid username: Username cannot be empty when saslMechanism is set")
	}
	if password == "" {
		return nil, errors.New("invalid password: Password cannot be empty when saslMechanism is set")
	}

	switch mechanism {
	case "PLAIN":
		return plain.Auth{User: username, Pass: password}.AsMechanism(), nil
	case "SCRAM-SHA-256":
		return scram.Auth{User: username, Pass: password}.AsSha256Mechanism(), nil
	case "SCRAM-SHA-512":
		return scram.Auth{User: username, Pass: password}.AsSha512Mechanism(), nil
	default:
		return nil, fmt.Errorf("invalid saslMechanism %q: supported mechanisms are PLAIN, SCRAM-SHA-256 and SCRAM-SHA-512", mechanism)
	}
}

// getTLSConfig returns nil when TLS is disabled. TLS is enabled unless the tls token is false.
func getTLSConfig(credentialTokens map[string]string) (*tls.Config, error) {
	enabled := true
	if value := strings.TrimSpace(credentialTokens[tlsToken]); value != "" {
		var err error
		if enabled, err = strconv.ParseBool(value); err != nil {
			return nil, fmt.Errorf("invalid tls %q: expected true or false", value)
		}
	}
	caCertificate := credentialTokens[tlsCACertificateToken]
	clientCertificate := credentialTokens[tlsClientCertificateToken]
	clientKey := credentialTokens[tlsClientKeyToken]
	serverName := credentialTokens[tlsServerNameToken]

	if !enabled {
		if caCertificate != "" || clientCertificate != "" || clientKey != "" || serverName != "" {
			return nil, errors.New("invalid tls: TLS options are set but tls is false")
		}
		return nil, nil
	}

	// kgo sets ServerName to the dialed broker host when it is empty.
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: serverName,
	}
	if caCertificate != "" {
		rootCAs := x509.NewCertPool()
		if !rootCAs.AppendCertsFromPEM([]byte(caCertificate)) {
			return nil, errors.New("invalid tlsCaCertificate: no PEM certificate found")
		}
		tlsConfig.RootCAs = rootCAs
	}
	if (clientCertificate == "") != (clientKey == "") {
		return nil, errors.New("invalid TLS client authentication: both tlsClientCertificate and tlsClientKey are required")
	}
	if clientCertificate != "" {
		certificate, err := tls.X509KeyPair([]byte(clientCertificate), []byte(clientKey))
		if err != nil {
			return nil, fmt.Errorf("invalid TLS client authentication: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	return tlsConfig, nil
}

type ConsumerGroup struct {
	Group string `json:"group"`
	State string `json:"state"`
	// GroupProtocol is "classic", or "consumer" for KIP-848 groups.
	GroupProtocol string        `json:"groupProtocol"`
	ProtocolType  string        `json:"protocolType,omitempty"`
	CoordinatorID *int32        `json:"coordinatorId,omitempty"`
	Members       []GroupMember `json:"members"`
}

type GroupMember struct {
	MemberID    string            `json:"memberId"`
	InstanceID  string            `json:"instanceId,omitempty"`
	ClientID    string            `json:"clientId"`
	ClientHost  string            `json:"clientHost"`
	Assignments []TopicPartitions `json:"assignments"`
}

type TopicPartitions struct {
	Topic      string  `json:"topic"`
	Partitions []int32 `json:"partitions"`
}

// describeGroup reports a group that does not exist as Dead.
func describeGroup(ctx context.Context, client adminClient, group string) (*ConsumerGroup, error) {
	// The classic API reports KIP-848 groups as missing, or without their
	// members, so ask the KIP-848 API first when the brokers support it.
	supported, err := client.SupportsConsumerGroupDescribe(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to describe consumer group %q: %w", group, err)
	}
	if supported {
		described, err := client.DescribeConsumerGroups(ctx, group)
		if err != nil {
			return nil, fmt.Errorf("unable to describe consumer group %q: %w", group, err)
		}
		g, ok := described[group]
		// The KIP-848 API reports classic groups as missing.
		if ok && g.Err != nil && !errors.Is(g.Err, kerr.GroupIDNotFound) {
			return nil, fmt.Errorf("unable to describe consumer group %q: %w", group, g.Err)
		}
		if ok && g.Err == nil && !strings.EqualFold(g.State, groupStateDead) {
			return newConsumerProtocolGroup(g), nil
		}
	}

	described, err := client.DescribeGroups(ctx, group)
	if err != nil {
		return nil, fmt.Errorf("unable to describe consumer group %q: %w", group, err)
	}
	classic, err := described.On(group, nil)
	if err == nil {
		err = classic.Err
	}
	if errors.Is(err, kerr.GroupIDNotFound) {
		return &ConsumerGroup{Group: group, State: groupStateDead, GroupProtocol: classicGroupProtocol, Members: []GroupMember{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("unable to describe consumer group %q: %w", group, err)
	}
	return newClassicConsumerGroup(classic), nil
}

func newClassicConsumerGroup(g kadm.DescribedGroup) *ConsumerGroup {
	coordinatorID := g.Coordinator.NodeID
	group := &ConsumerGroup{
		Group:         g.Group,
		State:         g.State,
		GroupProtocol: classicGroupProtocol,
		ProtocolType:  g.ProtocolType,
		CoordinatorID: &coordinatorID,
		Members:       make([]GroupMember, 0, len(g.Members)),
	}
	for _, m := range g.Members {
		member := GroupMember{
			MemberID:    m.MemberID,
			InstanceID:  derefString(m.InstanceID),
			ClientID:    m.ClientID,
			ClientHost:  m.ClientHost,
			Assignments: []TopicPartitions{},
		}
		if assigned, ok := m.Assigned.AsConsumer(); ok {
			for _, t := range assigned.Topics {
				member.Assignments = append(member.Assignments, TopicPartitions{Topic: t.Topic, Partitions: t.Partitions})
			}
		}
		group.Members = append(group.Members, member)
	}
	return group
}

func newConsumerProtocolGroup(g kadm.DescribedConsumerGroup) *ConsumerGroup {
	coordinatorID := g.Coordinator.NodeID
	group := &ConsumerGroup{
		Group:         g.Group,
		State:         g.State,
		GroupProtocol: consumerGroupProtocol,
		CoordinatorID: &coordinatorID,
		Members:       make([]GroupMember, 0, len(g.Members)),
	}
	for _, m := range g.Members {
		member := GroupMember{
			MemberID:    m.MemberID,
			InstanceID:  derefString(m.InstanceID),
			ClientID:    m.ClientID,
			ClientHost:  m.ClientHost,
			Assignments: []TopicPartitions{},
		}
		for _, t := range m.Assignment.Sorted() {
			member.Assignments = append(member.Assignments, TopicPartitions{Topic: t.Topic, Partitions: t.Partitions})
		}
		group.Members = append(group.Members, member)
	}
	return group
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
