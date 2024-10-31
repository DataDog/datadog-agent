// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression"

	//nolint:revive // TODO(AML) Fix revive linter
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

type benchmarkDeps struct {
	fx.In
	Log        log.Component
	Hostname   hostname.Component
	Compressor compression.Component
}

func benchmarkAddBucket(bucketValue int64, b *testing.B) {
	// Because these benchs can run for a long time, the aggregator is trying to
	// flush and because the serializer is not initialized it panics with a nil.
	// For some reasons using InitAggregator[WithInterval] doesn't fix the problem,
	// but this do.
	deps := fxutil.Test[benchmarkDeps](b, core.MockBundle())
	taggerComponent := fxutil.Test[tagger.Mock](b, taggerimpl.MockModule())
	forwarderOpts := forwarder.NewOptionsWithResolvers(pkgconfigsetup.Datadog(), deps.Log, resolver.NewSingleDomainResolvers(map[string][]string{"hello": {"world"}}))
	options := DefaultAgentDemultiplexerOptions()
	options.DontStartForwarders = true
	sharedForwarder := forwarder.NewDefaultForwarder(pkgconfigsetup.Datadog(), deps.Log, forwarderOpts)
	orchestratorForwarder := optional.NewOption[defaultforwarder.Forwarder](defaultforwarder.NoopForwarder{})
	eventPlatformForwarder := optional.NewOptionPtr[eventplatform.Forwarder](eventplatformimpl.NewNoopEventPlatformForwarder(deps.Hostname))
	demux := InitAndStartAgentDemultiplexer(deps.Log, sharedForwarder, &orchestratorForwarder, options, eventPlatformForwarder, deps.Compressor, taggerComponent, "hostname")
	defer demux.Stop(true)

	checkSampler := newCheckSampler(1, true, true, 1000, tags.NewStore(true, "bench"), checkid.ID("hello:world:1234"), taggerComponent)

	bucket := &metrics.HistogramBucket{
		Name:       "my.histogram",
		Value:      bucketValue,
		LowerBound: 10.0,
		UpperBound: 20.0,
		Tags:       []string{"foo", "bar"},
		Timestamp:  12345.0,
	}

	for n := 0; n < b.N; n++ {
		checkSampler.addBucket(bucket)
		// reset bucket cache
		checkSampler.lastBucketValue = make(map[ckey.ContextKey]int64)
	}
}

func benchmarkAddBucketWideBounds(bucketValue int64, b *testing.B) {
	taggerComponent := fxutil.Test[tagger.Mock](b, taggerimpl.MockModule())
	checkSampler := newCheckSampler(1, true, true, 1000, tags.NewStore(true, "bench"), checkid.ID("hello:world:1234"), taggerComponent)

	bounds := []float64{0, .0005, .001, .003, .005, .007, .01, .015, .02, .025, .03, .04, .05, .06, .07, .08, .09, .1, .5, 1, 5, 10}
	bucket := &metrics.HistogramBucket{
		Name:      "my.histogram",
		Value:     bucketValue,
		Tags:      []string{"foo", "bar"},
		Timestamp: 12345.0,
	}

	for n := 0; n < b.N; n++ {
		for i := range bounds {
			if i == 0 {
				continue
			}
			bucket.LowerBound = bounds[i-1]
			bucket.UpperBound = bounds[i]
			checkSampler.addBucket(bucket)
		}
		// reset bucket cache
		checkSampler.lastBucketValue = make(map[ckey.ContextKey]int64)
	}
}

func BenchmarkAddBucket1(b *testing.B)        { benchmarkAddBucket(1, b) }
func BenchmarkAddBucket10(b *testing.B)       { benchmarkAddBucket(10, b) }
func BenchmarkAddBucket100(b *testing.B)      { benchmarkAddBucket(100, b) }
func BenchmarkAddBucket1000(b *testing.B)     { benchmarkAddBucket(1000, b) }
func BenchmarkAddBucket10000(b *testing.B)    { benchmarkAddBucket(10000, b) }
func BenchmarkAddBucket1000000(b *testing.B)  { benchmarkAddBucket(1000000, b) }
func BenchmarkAddBucket10000000(b *testing.B) { benchmarkAddBucket(10000000, b) }

func BenchmarkAddBucketWide1e10(b *testing.B) { benchmarkAddBucketWideBounds(1e10, b) }
