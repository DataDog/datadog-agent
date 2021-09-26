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
	"syscall"
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

// NewSIGHUPSubscriber returns a new subscriber with the specified PID. A SIGHUP signal
// will be sent to this PID when a new configuration was fetched.
func NewSIGHUPSubscriber(product pbgo.Product, refreshRate time.Duration, pid int) *Subscriber {
	return NewSubscriber(product, refreshRate, func(config *pbgo.ConfigResponse) error {
		return syscall.Kill(pid, syscall.SIGHUP)
	})
}

// NewGRPCSubscriber returns a new gRPC stream based subscriber.
func NewGRPCSubscriber(product pbgo.Product, callback SubscriberCallback) error {
	creds := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})

	conn, err := grpc.DialContext(
		context.Background(),
		fmt.Sprintf(":%v", config.Datadog.GetInt("cmd_port")),
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return err
	}

	agentClient := pbgo.NewAgentSecureClient(conn)

	token, err := security.FetchAuthToken()
	if err != nil {
		err = fmt.Errorf("unable to fetch authentication token: %w", err)
		log.Infof("unable to establish stream, will possibly retry: %s", err)
		return err
	}

	currentConfigSnapshotVersion := uint64(0)
	client := tuf.NewDirectorPartialClient()

	go func() {
		log.Debug("Waiting for configuration from remote config management")

		streamCtx, streamCancel := context.WithCancel(
			metadata.NewOutgoingContext(context.Background(), metadata.MD{
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
				log.Error("Failed to request configuration, retrying in 3 seconds...")
				time.Sleep(3 * time.Second)
				continue
			}

			for {
				// Get new event from stream
				configResponse, err := stream.Recv()
				if err == io.EOF {
					continue
				} else if err != nil {
					log.Warnf("Stopped listening for configuration from remote config management: %s", err)
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

	return nil
}
