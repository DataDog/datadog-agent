// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package kafka provides a simple wrapper around 3rd party kafka client.
package kafka

import (
	"context"
	"fmt"
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
	DialFn        func(context.Context, string, string) (net.Conn, error)
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

	if opts.DialFn != nil {
		kafkaOptions = append(kafkaOptions, kgo.Dialer(opts.DialFn))
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
	_, err := adminClient.CreateTopics(ctxTimeout, 2, 1, nil, topicName)
	if err != nil {
		return err
	}

	if err := c.waitForLeaders(topicName); err != nil {
		return err
	}

	c.Client.ForceMetadataRefresh()

	// Block until metadata is fetched
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return c.Client.Ping(ctx)
}

func (c *Client) waitForLeaders(topicName string) error {
	admin := kadm.NewClient(c.Client)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for {
		meta, err := admin.Metadata(ctx, topicName)
		if err != nil {
			return err
		}
		topicMeta, ok := meta.Topics[topicName]
		if !ok {
			return fmt.Errorf("topic %s not found", topicName)
		}
		allReady := true
		for _, p := range topicMeta.Partitions {
			if p.Leader == -1 {
				allReady = false
				break
			}
		}
		if allReady {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}
