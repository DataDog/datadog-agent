// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote/service/tuf"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const errorRetryInterval = 3 * time.Second

// SubscriberCallback defines the function called when a new configuration was fetched
type SubscriberCallback func(config *pbgo.ConfigResponse) error

// Subscriber describes a product's configuration subscriber
type Subscriber struct {
	product     pbgo.Product
	refreshRate time.Duration
	lastUpdate  time.Time
	lastVersion uint64
	callback    SubscriberCallback
}

// NewSubscriber returns a new subscriber with the specified refresh rate and a callback
func NewSubscriber(product pbgo.Product, refreshRate time.Duration, callback SubscriberCallback) *Subscriber {
	return &Subscriber{
		product:     product,
		refreshRate: refreshRate,
		callback:    callback,
	}
}

// NewGRPCSubscriber returns a new gRPC stream based subscriber.
func NewGRPCSubscriber(product pbgo.Product, callback SubscriberCallback) (context.CancelFunc, error) {
	creds := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})

	ctx, cancel := context.WithCancel(context.Background())
	conn, err := grpc.DialContext(
		ctx,
		fmt.Sprintf(":%v", config.Datadog.GetInt("cmd_port")),
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		cancel()
		return nil, err
	}

	agentClient := pbgo.NewAgentSecureClient(conn)

	token, err := security.FetchAuthToken()
	if err != nil {
		cancel()
		err = fmt.Errorf("unable to fetch authentication token: %w", err)
		log.Infof("unable to establish stream, will possibly retry: %s", err)
		return nil, err
	}

	currentConfigSnapshotVersion := uint64(0)
	client := tuf.NewDirectorPartialClient()

	go func() {
		log.Debug("Waiting for configuration from remote config management")

		streamCtx, streamCancel := context.WithCancel(
			metadata.NewOutgoingContext(ctx, metadata.MD{
				"authorization": []string{fmt.Sprintf("Bearer %s", token)},
			}),
		)
		defer streamCancel()

		log.Debug("Processing response from remote config management")

		for {
			request := pbgo.SubscribeConfigRequest{
				CurrentConfigSnapshotVersion: currentConfigSnapshotVersion,
				Product:                      product,
			}
			stream, err := agentClient.GetConfigUpdates(streamCtx, &request)
			if err != nil {
				log.Errorf("Failed to request configuration, retrying in %s...", errorRetryInterval)
				time.Sleep(errorRetryInterval)
				continue
			}

			for {
				// Get new event from stream
				configResponse, err := stream.Recv()
				if err == io.EOF {
					continue
				} else if err != nil {
					log.Warnf("Stopped listening for configuration from remote config management: %s", err)
					time.Sleep(errorRetryInterval)
					break
				}

				if err := client.Verify(configResponse); err != nil {
					log.Errorf("Partial verify failed: %s", err)
					continue
				}

				log.Infof("Got config for product %s", product)
				if err := callback(configResponse); err == nil {
					currentConfigSnapshotVersion = configResponse.ConfigSnapshotVersion
				}
			}
		}
	}()

	return cancel, nil
}
