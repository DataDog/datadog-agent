// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"sync"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/trace/config/features"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
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
	tpsTargets         map[Signature]pb.TargetTPS
	mu                 sync.RWMutex // protects concurrent access to samplers and tpsTargets
	tpsVersion         uint64       // version of the loaded tpsTargets
	duplicateTargetTPS uint64       // count of duplicate received targetTPS

	client  *remote.Client
	stopped chan struct{}
}

type remoteSampler struct {
	Sampler
	target pb.TargetTPS
}

func newRemoteRates(maxTPS float64) *RemoteRates {
	if !features.Has("remote_rates") {
		return nil
	}
	client, err := remote.NewClient(remote.Facts{ID: "trace-agent", Name: "trace-agent", Version: version.AgentVersion}, []data.Product{data.ProductAPMSampling})
	if err != nil {
		log.Errorf("Error when subscribing to remote config management %v", err)
		return nil
	}
	return &RemoteRates{
		client:    client,
		maxSigTPS: maxTPS,
		samplers:  make(map[Signature]*remoteSampler),
		stopped:   make(chan struct{}),
	}
}

func (r *RemoteRates) onUpdate(update remote.APMSamplingUpdate) error {
	log.Debugf("fetched config version %d from remote config management", update.Config.Version)
	tpsTargets := make(map[Signature]pb.TargetTPS, len(r.tpsTargets))
	for _, rates := range update.Config.Rates {
		for _, targetTPS := range rates.TargetTPS {
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
	atomic.StoreUint64(&r.tpsVersion, update.Config.Version)
	return nil
}

// addTargetTPS keeping the highest rank if 2 targetTPS of the same signature are added
func (r *RemoteRates) addTargetTPS(tpsTargets map[Signature]pb.TargetTPS, new pb.TargetTPS) {
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
		atomic.AddUint64(&r.duplicateTargetTPS, 1)
	}
}

func (r *RemoteRates) updateTPS(tpsTargets map[Signature]pb.TargetTPS) {
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
		sampler.UpdateTargetTPS(target.Value)
	}
	r.mu.RUnlock()

	// trim signatures with no TPS configured
	r.mu.Lock()
	for sig := range noTPSConfigured {
		delete(r.samplers, sig)
	}
	r.mu.Unlock()
}

// update all samplers
func (r *RemoteRates) update() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.samplers {
		s.update()
	}
}

// Start runs and adjust rates per signature following remote TPS targets
func (r *RemoteRates) Start() {
	go func() {
		for update := range r.client.APMSamplingUpdates() {
			r.onUpdate(update)
		}
		close(r.stopped)
	}()
}

// Stop stops RemoteRates main loop
func (r *RemoteRates) Stop() {
	r.client.Close()
	<-r.stopped
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
		*s,
		targetTPS,
	}
	r.mu.Lock()
	r.samplers[sig] = sampler
	r.mu.Unlock()
	return sampler, true
}

// CountSignature counts the number of root span seen matching a signature.
func (r *RemoteRates) CountSignature(sig Signature) {
	s, ok := r.getSampler(sig)
	if !ok {
		if s, ok = r.initSampler(sig); !ok {
			return
		}
	}
	s.Backend.CountSignature(sig)
}

// CountSample counts the number of sampled root span matching a signature.
func (r *RemoteRates) CountSample(root *pb.Span, sig Signature) {
	s, ok := r.getSampler(sig)
	if !ok {
		return
	}
	s.Backend.CountSample()
	root.Metrics[tagRemoteTPS] = s.targetTPS.Load()
	root.Metrics[tagRemoteVersion] = float64(atomic.LoadUint64(&r.tpsVersion))
	return
}

// CountWeightedSig counts weighted root span seen for a signature.
// This function is called when trace-agent client drop unsampled spans.
// as dropped root spans are not accounted anymore in CountSignature calls.
func (r *RemoteRates) CountWeightedSig(sig Signature, weight float64) {
	s, ok := r.getSampler(sig)
	if !ok {
		return
	}
	s.Backend.CountWeightedSig(sig, weight)
	s.Backend.AddTotalScore(weight)
}

// GetSignatureSampleRate returns the sampling rate to apply for a registered signature.
func (r *RemoteRates) GetSignatureSampleRate(sig Signature) (float64, bool) {
	s, ok := r.getSampler(sig)
	if !ok {
		return 0, false
	}
	return s.GetSignatureSampleRate(sig), true
}

// getAllSignatureSampleRates returns sampling rates to apply for all registered signatures.
func (r *RemoteRates) getAllSignatureSampleRates() map[Signature]rm {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make(map[Signature]rm, len(r.samplers))
	for sig, s := range r.samplers {
		res[sig] = rm{
			r: s.GetSignatureSampleRate(sig),
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
	if duplicateTargetTPS := atomic.SwapUint64(&r.duplicateTargetTPS, 0); duplicateTargetTPS != 0 {
		metrics.Count("datadog.trace_agent.remote.duplicate_target_tps", int64(duplicateTargetTPS), nil, 1)
	}
}
