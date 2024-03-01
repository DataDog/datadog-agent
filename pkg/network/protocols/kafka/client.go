// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package kafka provides a simple wrapper around 3rd party kafka client.
package kafka

import (
	"context"
	"net"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	defaultTimeout = time.Second * 10
)

// Options is a struct to hold the options for the kafka client
type Options struct {
	ServerAddress string
	Dialer        *net.Dialer
	CustomOptions []kgo.Opt
}

// Client is a wrapper around the kafka client
type Client struct {
	Client *kgo.Client
}

// NewClient creates a new kafka client
func NewClient(opts Options) (*Client, error) {
	kafkaOptions := []kgo.Opt{kgo.SeedBrokers(opts.ServerAddress)}
	kafkaOptions = append(kafkaOptions, opts.CustomOptions...)
	if opts.Dialer != nil {
		kafkaOptions = append(kafkaOptions, kgo.Dialer(opts.Dialer.DialContext))
	}
	client, err := kgo.NewClient(kafkaOptions...)
	if err != nil {
		return nil, err
	}

	ctxTimeout, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	if err := client.Ping(ctxTimeout); err != nil {
		return nil, err
	}

	return &Client{
		Client: client,
	}, nil
}

// CreateTopic creates a topic named topicName.
func (c *Client) CreateTopic(topicName string) error {
	adminClient := kadm.NewClient(c.Client)
	ctxTimeout, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	_, err := adminClient.CreateTopics(ctxTimeout, 1, 1, nil, topicName)
	return err
}
