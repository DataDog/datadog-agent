// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config"
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

// NewChanSubscriber returns a new subscriber that will put the ConfigResponse objects onto a provided channel.
func NewChanSubscriber(product pbgo.Product, refreshRate time.Duration, channel chan *pbgo.ConfigResponse) *Subscriber {
	return NewSubscriber(product, refreshRate, func(config *pbgo.ConfigResponse) error {
		select {
		case channel <- config:
			return nil
		default:
			return errors.New("failed to put config onto channel")
		}
	})
}

// NewGRPCSubscriber returns a new gRPC stream based subscriber.
func NewGRPCSubscriber(product pbgo.Product, callback SubscriberCallback) (context.CancelFunc, error) {
	currentConfigSnapshotVersion := uint64(0)

	ctx, cancel := context.WithCancel(context.Background())

	token, err := security.FetchAuthToken()
	if err != nil {
		cancel()
		err = fmt.Errorf("unable to fetch authentication token: %w", err)
		log.Infof("unable to establish stream, will possibly retry: %s", err)
		return nil, err
	}

	creds := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})
	conn, err := grpc.DialContext(
		ctx,
		fmt.Sprintf(":%v", config.Datadog.GetInt("cmd_port")),
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		cancel()
		return nil, err
	}

	streamCtx := metadata.NewOutgoingContext(ctx, metadata.MD{
		"authorization": []string{fmt.Sprintf("Bearer %s", token)},
	})

	subscriberStream, err := newSubscriberStream(streamCtx, conn)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create core agent stream: %w", err)
	}

	go func() {
		log.Debug("Waiting for configuration from remote config management")
		for {
			request := pbgo.SubscribeConfigRequest{
				CurrentConfigSnapshotVersion: currentConfigSnapshotVersion,
				Product:                      product,
			}
			subscriberStream.startStream(streamCtx)
			err := subscriberStream.stream.Send(&request)
			if err != nil {
				log.Errorf("Error sending message to core agent: %s", err)
				time.Sleep(errorRetryInterval)
				continue
			}
			subscriberStream.readConfigs(streamCtx, product, callback)
		}
	}()

	return cancel, nil
}

// NewTracerGRPCSubscriber returns a new gRPC stream based subscriber. The subscriber sends tracer infos to core agent
// and listens for configuration updates from core agent asynchronously.
func NewTracerGRPCSubscriber(product pbgo.Product, callback SubscriberCallback, tracerInfos chan *pbgo.TracerInfo) (context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(context.Background())

	token, err := security.FetchAuthToken()
	if err != nil {
		cancel()
		err = fmt.Errorf("unable to fetch authentication token: %w", err)
		log.Infof("unable to establish stream, will possibly retry: %s", err)
		return nil, err
	}

	streamCtx := metadata.NewOutgoingContext(ctx, metadata.MD{
		"authorization": []string{fmt.Sprintf("Bearer %s", token)},
	})

	creds := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})
	conn, err := grpc.DialContext(
		ctx,
		fmt.Sprintf(":%v", config.Datadog.GetInt("cmd_port")),
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		cancel()
		return nil, err
	}

	subscriberStream, err := newSubscriberStream(streamCtx, conn)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create core agent stream: %w", err)
	}

	go subscriberStream.sendTracerInfos(streamCtx, tracerInfos, product)
	go subscriberStream.readConfigs(streamCtx, product, callback)

	return cancel, nil
}
