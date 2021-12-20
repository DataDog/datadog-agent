package service

import (
	"context"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type subscriberStream struct {
	mx                            sync.Mutex
	rwmx                          sync.RWMutex
	stream                        pbgo.AgentSecure_GetConfigUpdatesClient
	agentClient                   pbgo.AgentSecureClient
	currentConfigSnapshotVersions map[pbgo.Product]uint64
	tufClient                     *uptane.PartialClient
}

func newSubscriberStream(streamCtx context.Context, conn *grpc.ClientConn) (*subscriberStream, error) {
	c := &subscriberStream{
		agentClient:                   pbgo.NewAgentSecureClient(conn),
		tufClient:                     uptane.NewPartialClient(),
		currentConfigSnapshotVersions: make(map[pbgo.Product]uint64),
	}
	c.startStream(streamCtx)
	return c, nil
}

func (c *subscriberStream) startStream(streamCtx context.Context) {
	var stream pbgo.AgentSecure_GetConfigUpdatesClient
	var err error
	for {
		stream, err = c.agentClient.GetConfigUpdates(streamCtx)
		if err != nil {
			log.Errorf("Failed to establish channel to core-agent, retrying in %s...", errorRetryInterval)
			time.Sleep(errorRetryInterval)
			continue
		} else {
			log.Debugf("Successfully established channel to core-agent")
			break
		}
	}
	c.mx.Lock()
	defer c.mx.Unlock()
	c.stream = stream
}

func (c *subscriberStream) sendTracerInfos(streamCtx context.Context, tracerInfos chan *pbgo.TracerInfo, product pbgo.Product) {
	for {
		select {
		case tracerInfo := <-tracerInfos:
			request := pbgo.SubscribeConfigRequest{
				CurrentConfigSnapshotVersion: c.getProductSnapshotsVersion(product),
				Product:                      product,
				TracerInfo:                   tracerInfo,
			}
			log.Trace("Sending subscribe config requests with tracer infos to core-agent")
			if err := c.stream.Send(&request); err != nil {
				log.Warnf("Error writing tracer infos to stream: %s", err)
				time.Sleep(errorRetryInterval)
				c.startStream(streamCtx)
				continue
			}
		case <-streamCtx.Done():
			return
		}
	}
}

func (c *subscriberStream) readConfigs(streamCtx context.Context, product pbgo.Product, callback SubscriberCallback) {
	for {
		log.Debug("Waiting for new config")
		configResponse, err := c.stream.Recv()
		if err != nil {
			log.Warnf("Stopped listening for configuration from remote config management: %s", err)
			time.Sleep(errorRetryInterval)
			c.startStream(streamCtx)
			continue
		}

		if err := c.tufClient.Verify(configResponse); err != nil {
			log.Errorf("Partial verify failed: %s", err)
			continue
		}

		log.Infof("Got config for product %s", product)
		if err := callback(configResponse); err == nil {
			c.setProductSnapshotsVersion(product, configResponse.ConfigSnapshotVersion)
		}

		select {
		case <-streamCtx.Done():
			return
		default:
			continue
		}
	}
}

func (c *subscriberStream) setProductSnapshotsVersion(product pbgo.Product, version uint64) {
	c.rwmx.Lock()
	defer c.rwmx.Unlock()
	c.currentConfigSnapshotVersions[product] = version
}

func (c *subscriberStream) getProductSnapshotsVersion(product pbgo.Product) uint64 {
	c.rwmx.RLock()
	defer c.rwmx.RUnlock()
	return c.currentConfigSnapshotVersions[product]
}
