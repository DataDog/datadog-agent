// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"maps"
	"net/http"
	"slices"
	"strconv"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
)

// Filterable defines the minimal interface needed for pipeline
// filtering. Both Series and SketchSeries implement this interface.
type Filterable interface {
	GetName() string
}

// Filter can decide if metric should be included in a payload or not.
type Filter interface {
	// Filter returns true when metric should be added to the payload.
	Filter(Filterable) bool
}

// AllowAllFilter admits all metrics into the payload.
type AllowAllFilter struct{}

// Filter implements Filter interface.
func (nf AllowAllFilter) Filter(_ Filterable) bool {
	return true
}

// MapFilter admits only metrics whose name is present in the map.
type MapFilter struct {
	m *map[string]struct{}
}

// NewMapFilter creates a new filter using the supplied map.
//
// The map is not copied and is shared with the filter.
func NewMapFilter(m map[string]struct{}) MapFilter {
	return MapFilter{&m}
}

// Filter implements Filter interface.
func (mf MapFilter) Filter(f Filterable) bool {
	_, ok := (*mf.m)[f.GetName()]
	return ok
}

// ToList returns a sorted list of allowed metric names.
func (mf MapFilter) ToList() []string {
	return slices.Sorted(maps.Keys(*mf.m))
}

// PipelineConfig contains properties that determine how a payload is
// built.
//
// Put here everything that makes a payload different from other
// payloads. Conversly, if it doesn't change bytes of the payload, it
// doesn't belong here.
//
// This type must be comparable.
type PipelineConfig struct {
	Filter Filter
	V3     bool
}

// PipelineDestination describes how to deliver a payload to the intake.
type PipelineDestination struct {
	Resolver             resolver.DomainResolver
	Endpoint             transaction.Endpoint
	AddValidationHeaders bool
}

type forwarder interface {
	SubmitTransaction(*transaction.HTTPTransaction) error
}

func (dest *PipelineDestination) send(payloads transaction.BytesPayloads, forwarder forwarder, headers http.Header) error {
	batchID, err := dest.maybeMakeBatchID()
	if err != nil {
		return err
	}

	domain := dest.Resolver.Resolve(dest.Endpoint)
	for _, auth := range dest.Resolver.GetAuthorizers() {
		for seq, payload := range payloads {
			txn := transaction.NewHTTPTransaction()
			txn.Domain = domain
			txn.Endpoint = dest.Endpoint
			txn.Payload = payload
			for key := range headers {
				txn.Headers.Set(key, headers.Get(key))
			}
			if dest.AddValidationHeaders {
				txn.Headers.Set("X-Metrics-Request-ID", batchID)
				txn.Headers.Set("X-Metrics-Request-Seq", strconv.Itoa(seq))
				txn.Headers.Set("X-Metrics-Request-Len", strconv.Itoa(len(payloads)))
			}

			auth.Authorize(txn)
			err := forwarder.SubmitTransaction(txn)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (dest *PipelineDestination) maybeMakeBatchID() (string, error) {
	if dest.AddValidationHeaders {
		uuid, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		return uuid.String(), nil
	}
	return "", nil
}

// PipelineContext holds information needed during and after pipeline execution.
type PipelineContext struct {
	Destinations []PipelineDestination
	payloads     transaction.BytesPayloads
}

func (c *PipelineContext) addPayload(p *transaction.BytesPayload) {
	c.payloads = append(c.payloads, p)
}

func (c *PipelineContext) send(forwarder forwarder, headers http.Header) error {
	for _, dest := range c.Destinations {
		if err := dest.send(c.payloads, forwarder, headers); err != nil {
			return err
		}
	}
	return nil
}

// PipelineSet is a collection of pipelines, mapping unique descriptors to their contexts.
type PipelineSet map[PipelineConfig]*PipelineContext

// Add adds a resolver to a pipeline with matching configuration or creates a new one.
func (ps PipelineSet) Add(conf PipelineConfig, dest PipelineDestination) {
	p := ps[conf]
	if p == nil {
		p = &PipelineContext{}
		ps[conf] = p
	}

	p.Destinations = append(p.Destinations, dest)
}

// Send out payloads accumulated by the pipelines to their destinations.
func (ps PipelineSet) Send(forwarder forwarder, headers http.Header) error {
	for _, ctx := range ps {
		if err := ctx.send(forwarder, headers); err != nil {
			return err
		}
	}
	return nil
}
