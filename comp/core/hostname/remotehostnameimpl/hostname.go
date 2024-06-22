// Package remotehostnameimpl provides a function to get the hostname from core agent.
package remotehostnameimpl

import (
	"context"
	"time"

	"github.com/avast/retry-go/v4"

	"github.com/DataDog/datadog-agent/pkg/config"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// maxAttempts is the maximum number of times we try to get the hostname
	// from the core-agent before bailing out.
	maxAttempts = 6
)

type remotehostimpl struct {
	hostname string
}

func (r *remotehostimpl) Get(ctx context.Context) (string, error) {
	if r.hostname != "" {
		return r.hostname, nil
	}
	hostname, err := utils.GetHostnameWithContextAndFallback(ctx)
	if err != nil {
		return "", err
	}
	r.hostname = hostname
	return hostname, nil
}

func (r *remotehostimpl) GetSafe(ctx context.Context) string {
	h, _ := r.Get(ctx)
	return h
}

func (r *remotehostimpl) GetWithProvider(ctx context.Context) (hostnameinterface.Data, error) {
	h, err := r.Get(ctx)
	if err != nil {
		return hostnameinterface.Data{}, err
	}
	return hostnameinterface.Data{
		Hostname: h,
		Provider: "remote",
	}, nil
}

// getHostnameWithContext attempts to acquire a hostname by connecting to the
// core agent's gRPC endpoints extending the given context.
func getHostnameWithContext(ctx context.Context) (string, error) {
	var hostname string
	err := retry.Do(func() error {
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		ipcAddress, err := config.GetIPCAddress()
		if err != nil {
			return err
		}

		client, err := grpc.GetDDAgentClient(ctx, ipcAddress, config.GetIPCPort())
		if err != nil {
			return err
		}

		reply, err := client.GetHostname(ctx, &pbgo.HostnameRequest{})
		if err != nil {
			return err
		}

		log.Debugf("Acquired hostname from gRPC: %s", reply.Hostname)

		hostname = reply.Hostname
		return nil
	}, retry.LastErrorOnly(true), retry.Attempts(maxAttempts), retry.Context(ctx))
	return hostname, err
}

// getHostnameWithContextAndFallback attempts to acquire a hostname by connecting to the
// core agent's gRPC endpoints extending the given context, or falls back to local resolution
func getHostnameWithContextAndFallback(ctx context.Context) (string, error) {
	hostnameDetected, err := getHostnameWithContext(ctx)
	if err != nil {
		log.Warnf("Could not resolve hostname from core-agent: %v", err)
		hostnameDetected, err = hostname.Get(ctx)
		if err != nil {
			return "", err
		}
	}
	log.Infof("Hostname is: %s", hostnameDetected)
	return hostnameDetected, nil
}
