// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state/products/apmsampling"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"go.uber.org/atomic"
)

const (
	tagRemoteTPS     = "_dd.remote.tps"
	tagRemoteVersion = "_dd.remote.version"
)

// RemoteRates computes rates per (env, service) to apply in trace-agent clients.
// The rates are adjusted to match a targetTPS per (env, service) received
// from remote configurations. RemoteRates listens for new remote configurations
// with a grpc subscriber. On reception, new tps targets replace the previous ones.
type RemoteRates struct {
	maxSigTPS float64
	// samplers contains active sampler adjusting rates to match latest tps targets
	// available. A sampler is added only if a span matching the signature is seen.
	samplers map[Signature]*remoteSampler
	// tpsTargets contains the latest tps targets available per (env, service)
	// this map may include signatures (env, service) not seen by this agent.
	tpsTargets         map[Signature]apmsampling.TargetTPS
	mu                 sync.RWMutex   // protects concurrent access to samplers and tpsTargets
	tpsVersion         *atomic.Uint64 // version of the loaded tpsTargets
	duplicateTargetTPS *atomic.Uint64 // count of duplicate received targetTPS

	client config.RemoteClient
}

type remoteSampler struct {
	*Sampler
	target apmsampling.TargetTPS
}

func newRemoteRates(client config.RemoteClient, maxTPS float64, agentVersion string) *RemoteRates {
	if client == nil {
		return nil
	}
	return &RemoteRates{
		client:             client,
		maxSigTPS:          maxTPS,
		samplers:           make(map[Signature]*remoteSampler),
		tpsVersion:         atomic.NewUint64(0),
		duplicateTargetTPS: atomic.NewUint64(0),
	}
}

func (r *RemoteRates) onUpdate(update map[string]state.APMSamplingConfig) {
	// TODO: We don't have a version per product, yet. But, we will have it in the next version.
	// In the meantime we will just use a version of one of the config files.
	var version uint64
	for _, c := range update {
		if c.Metadata.Version > version {
			version = c.Metadata.Version
		}
		break
	}

	log.Debugf("fetched config version %d from remote config management", version)
	tpsTargets := make(map[Signature]apmsampling.TargetTPS, len(r.tpsTargets))
	for _, rates := range update {
		for _, targetTPS := range rates.Config.TargetTPS {
			if targetTPS.Value > r.maxSigTPS {
				targetTPS.Value = r.maxSigTPS
			}
			if targetTPS.Value == 0 {
				continue
			}
			r.addTargetTPS(tpsTargets, targetTPS)
		}
	}
	r.updateTPS(tpsTargets)
	r.tpsVersion.Store(version)
}

// addTargetTPS keeping the highest rank if 2 targetTPS of the same signature are added
func (r *RemoteRates) addTargetTPS(tpsTargets map[Signature]apmsampling.TargetTPS, new apmsampling.TargetTPS) {
	sig := ServiceSignature{Name: new.Service, Env: new.Env}.Hash()
	stored, ok := tpsTargets[sig]
	if !ok {
		tpsTargets[sig] = new
		return
	}
	if new.Rank > stored.Rank {
		tpsTargets[sig] = new
		return
	}
	if new.Rank == stored.Rank {
		r.duplicateTargetTPS.Inc()
	}
}

func (r *RemoteRates) updateTPS(tpsTargets map[Signature]apmsampling.TargetTPS) {
	r.mu.Lock()
	r.tpsTargets = tpsTargets
	r.mu.Unlock()

	// update samplers with new TPS
	r.mu.RLock()
	noTPSConfigured := map[Signature]struct{}{}
	for sig, sampler := range r.samplers {
		target, ok := tpsTargets[sig]
		if !ok {
			noTPSConfigured[sig] = struct{}{}
		}
		sampler.target = target
		sampler.updateTargetTPS(target.Value)
	}
	r.mu.RUnlock()

	// trim signatures with no TPS configured
	r.mu.Lock()
	for sig := range noTPSConfigured {
		delete(r.samplers, sig)
	}
	r.mu.Unlock()
}

// Start runs and adjust rates per signature following remote TPS targets
func (r *RemoteRates) Start() {
	// Make sure the remote client is running, if the client is already started
	// this is a nop so this is fine.
	r.client.Start()
	r.client.RegisterAPMUpdate(r.onUpdate)
}

// Stop stops RemoteRates main loop
func (r *RemoteRates) Stop() {
	r.client.Close()
}

func (r *RemoteRates) getSampler(sig Signature) (*remoteSampler, bool) {
	r.mu.RLock()
	s, ok := r.samplers[sig]
	r.mu.RUnlock()
	return s, ok
}

func (r *RemoteRates) initSampler(sig Signature) (*remoteSampler, bool) {
	r.mu.RLock()
	targetTPS, ok := r.tpsTargets[sig]
	r.mu.RUnlock()
	if !ok {
		return nil, false
	}
	s := newSampler(1.0, targetTPS.Value, nil)
	sampler := &remoteSampler{
		s,
		targetTPS,
	}
	r.mu.Lock()
	r.samplers[sig] = sampler
	r.mu.Unlock()
	return sampler, true
}

// countWeightedSig counts the number of root span seen matching a signature.
func (r *RemoteRates) countWeightedSig(now time.Time, sig Signature, weight float32) {
	s, ok := r.getSampler(sig)
	if !ok {
		if s, ok = r.initSampler(sig); !ok {
			return
		}
	}
	s.countWeightedSig(now, sig, weight)
}

// countSample counts the number of sampled root span matching a signature.
func (r *RemoteRates) countSample(root *pb.Span, sig Signature) {
	s, ok := r.getSampler(sig)
	if !ok {
		return
	}
	s.countSample()
	root.Metrics[tagRemoteTPS] = s.targetTPS.Load()
	root.Metrics[tagRemoteVersion] = float64(r.tpsVersion.Load())
}

// getSignatureSampleRate returns the sampling rate to apply for a registered signature.
func (r *RemoteRates) getSignatureSampleRate(sig Signature) (float64, bool) {
	s, ok := r.getSampler(sig)
	if !ok {
		return 0, false
	}
	return s.getSignatureSampleRate(sig), true
}

// getAllSignatureSampleRates returns sampling rates to apply for all registered signatures.
func (r *RemoteRates) getAllSignatureSampleRates() map[Signature]rm {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make(map[Signature]rm, len(r.samplers))
	for sig, s := range r.samplers {
		res[sig] = rm{
			r: s.getSignatureSampleRate(sig),
			m: s.target.Mechanism,
		}
	}
	return res
}

func (r *RemoteRates) report() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	metrics.Gauge("datadog.trace_agent.remote.samplers", float64(len(r.samplers)), nil, 1)
	metrics.Gauge("datadog.trace_agent.remote.sig_targets", float64(len(r.tpsTargets)), nil, 1)
	if duplicateTargetTPS := r.duplicateTargetTPS.Swap(0); duplicateTargetTPS != 0 {
		metrics.Count("datadog.trace_agent.remote.duplicate_target_tps", int64(duplicateTargetTPS), nil, 1)
	}
}
