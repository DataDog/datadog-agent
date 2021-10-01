package sampler

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/trace/config/features"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	// samplers contains active sampler adjusting rates to match latest tps targets
	// available. A sampler is added only if a span matching the signature is seen.
	samplers map[Signature]*Sampler
	// tpsTargets contains the latest tps targets available per (env, service)
	// this map may include signatures (env, service) not seen by this agent.
	tpsTargets map[Signature]float64
	mu         sync.RWMutex // protects concurrent access to samplers and tpsTargets
	tpsVersion uint64       // version of the loaded tpsTargets

	stopSubscriber context.CancelFunc
	exit           chan struct{}
	stopped        chan struct{}
}

func newRemoteRates() *RemoteRates {
	if !features.Has("remote_rates") {
		return nil
	}
	remoteRates := &RemoteRates{
		samplers: make(map[Signature]*Sampler),
		exit:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}
	close, err := service.NewGRPCSubscriber(pbgo.Product_APM_SAMPLING, remoteRates.loadNewConfig)
	if err != nil {
		log.Errorf("Error when subscribing to remote config management %v", err)
		return nil
	}
	remoteRates.stopSubscriber = close
	return remoteRates
}

func (r *RemoteRates) loadNewConfig(new *pbgo.ConfigResponse) error {
	log.Debugf("fetched config version %d from remote config management", new.ConfigDelegatedTargetVersion)
	tpsTargets := make(map[Signature]float64, len(r.tpsTargets))
	for _, targetFile := range new.TargetFiles {
		var new pb.APMSampling
		_, err := new.UnmarshalMsg(targetFile.Raw)
		if err != nil {
			return err
		}
		for _, targetTPS := range new.TargetTps {
			sig := ServiceSignature{Name: targetTPS.Service, Env: targetTPS.Env}.Hash()
			tpsTargets[sig] = targetTPS.Value
		}
	}
	r.updateTPS(tpsTargets)
	atomic.StoreUint64(&r.tpsVersion, new.ConfigDelegatedTargetVersion)
	return nil
}

func (r *RemoteRates) updateTPS(tpsTargets map[Signature]float64) {
	r.mu.Lock()
	r.tpsTargets = tpsTargets
	r.mu.Unlock()

	// update samplers with new TPS
	r.mu.RLock()
	noTPSConfigured := map[Signature]struct{}{}
	for sig, sampler := range r.samplers {
		rate, ok := tpsTargets[sig]
		if !ok {
			noTPSConfigured[sig] = struct{}{}
		}
		sampler.UpdateTargetTPS(rate)
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
	go func() {
		defer watchdog.LogOnPanic()
		decayTicker := time.NewTicker(defaultDecayPeriod)
		adjustTicker := time.NewTicker(adjustPeriod)
		statsTicker := time.NewTicker(10 * time.Second)
		defer decayTicker.Stop()
		defer adjustTicker.Stop()
		defer statsTicker.Stop()
		for {
			select {
			case <-decayTicker.C:
				r.DecayScores()
			case <-adjustTicker.C:
				r.AdjustScoring()
			case <-statsTicker.C:
				r.report()
			case <-r.exit:
				close(r.stopped)
				return
			}
		}
	}()
}

// DecayScores decays scores of all samplers
func (r *RemoteRates) DecayScores() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.samplers {
		s.Backend.DecayScore()
	}
}

// AdjustScoring adjust scores of all samplers
func (r *RemoteRates) AdjustScoring() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.samplers {
		s.AdjustScoring()
	}
}

// Stop stops RemoteRates main loop
func (r *RemoteRates) Stop() {
	close(r.exit)
	r.stopSubscriber()
	<-r.stopped
}

func (r *RemoteRates) getSampler(sig Signature) (*Sampler, bool) {
	r.mu.RLock()
	s, ok := r.samplers[sig]
	r.mu.RUnlock()
	return s, ok
}

func (r *RemoteRates) initSampler(sig Signature) (*Sampler, bool) {
	r.mu.RLock()
	targetTPS, ok := r.tpsTargets[sig]
	r.mu.RUnlock()
	if !ok {
		return nil, false
	}
	s := newSampler(1.0, targetTPS, nil)
	r.mu.Lock()
	r.samplers[sig] = s
	r.mu.Unlock()
	return s, true
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

// GetAllSignatureSampleRates returns sampling rates to apply for all registered signatures.
func (r *RemoteRates) GetAllSignatureSampleRates() map[Signature]float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make(map[Signature]float64, len(r.samplers))
	for sig, s := range r.samplers {
		res[sig] = s.GetSignatureSampleRate(sig)
	}
	return res
}

func (r *RemoteRates) report() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	metrics.Gauge("datadog.trace_agent.remote.samplers", float64(len(r.samplers)), nil, 1)
	metrics.Gauge("datadog.trace_agent.remote.sig_targets", float64(len(r.tpsTargets)), nil, 1)
}
