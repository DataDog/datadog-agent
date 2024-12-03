// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tags holds tags related files
package tags

import (
	"context"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/remote"
	taggerTelemetry "github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Tagger defines a Tagger for the Tags Resolver
type Tagger interface {
	Start(ctx context.Context) error
	Stop() error
	Tag(entity string, cardinality types.TagCardinality) ([]string, error)
}

type nullTagger struct{}

func (n *nullTagger) Start(_ context.Context) error {
	return nil
}

func (n *nullTagger) Stop() error {
	return nil
}

func (n *nullTagger) Tag(_ string, _ types.TagCardinality) ([]string, error) {
	return nil, nil
}

// Resolver represents a cache resolver
type Resolver interface {
	Start(ctx context.Context) error
	Stop() error
	Resolve(id string) []string
	ResolveWithErr(id string) ([]string, error)
	GetValue(id string, tag string) string
}

// DefaultResolver represents a default resolver based directly on the underlying tagger
type DefaultResolver struct {
	tagger Tagger
}

// Start the resolver
func (t *DefaultResolver) Start(ctx context.Context) error {
	go func() {
		if err := t.tagger.Start(ctx); err != nil {
			log.Errorf("failed to init tagger: %s", err)
		}
	}()

	go func() {
		<-ctx.Done()
		_ = t.tagger.Stop()
	}()

	return nil
}

// Resolve returns the tags for the given id
func (t *DefaultResolver) Resolve(id string) []string {
	tags, _ := t.ResolveWithErr(id)
	return tags
}

// ResolveWithErr returns the tags for the given id
func (t *DefaultResolver) ResolveWithErr(id string) ([]string, error) {
	entityID := types.NewEntityID(types.ContainerID, id)
	return t.tagger.Tag(entityID.String(), types.OrchestratorCardinality)
}

// GetValue return the tag value for the given id and tag name
func (t *DefaultResolver) GetValue(id string, tag string) string {
	return utils.GetTagValue(tag, t.Resolve(id))
}

// Stop the resolver
func (t *DefaultResolver) Stop() error {
	return t.tagger.Stop()
}

// NewResolver returns a new tags resolver
func NewResolver(config *config.Config, telemetry telemetry.Component) Resolver {
	if config.RemoteTaggerEnabled {
		tagger, err := CreateRemoteTagger(telemetry)
		if err != nil {
			log.Errorf("unable to configure the remote tagger: %s", err)
		} else {
			return &DefaultResolver{
				tagger: tagger,
			}
		}
	}
	return &DefaultResolver{
		tagger: &nullTagger{},
	}
}

var (
	rts    sync.Once
	tagger *remote.Tagger
	err    error
)

// CreateRemoteTagger returns a new remote tagger component (TODO: use the fx component instead)
func CreateRemoteTagger(telemetry telemetry.Component) (*remote.Tagger, error) {
	rts.Do(func() {
		tagger, err = createRemoteTagger(telemetry)
	})
	return tagger, err
}

func createRemoteTagger(telemetry telemetry.Component) (*remote.Tagger, error) {
	options, err := remote.NodeAgentOptionsForSecurityResolvers(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, err
	}

	return remote.NewTagger(options, pkgconfigsetup.Datadog(), taggerTelemetry.NewStore(telemetry)), nil
}
