// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// FFADEConfig configures the F-FADE processor.
// F-FADE (Frequency Factorization for Anomaly Detection in Edge Streams)
// detects anomalies based on interaction frequency patterns using matrix factorization.
//
// Paper: "F-FADE: Frequency Factorization for Anomaly Detection in Edge Streams"
// (Shah et al., CIKM 2020) https://arxiv.org/pdf/2011.04723
type FFADEConfig struct {
	// Matrix factorization parameters
	LatentDim      int     // Number of latent dimensions (k), default: 10
	LearningRate   float64 // SGD learning rate (η), default: 0.01
	Regularization float64 // L2 regularization weight (α), default: 0.001
	SGDIterations  int     // SGD iterations per model update, default: 10

	// ITFM (Interaction Time-Frequency Matrix) parameters
	DecayRate    float64 // Temporal decay rate for frequencies, default: 0.001
	MinFrequency float64 // Eviction threshold for low-frequency edges, default: 0.01

	// Model update parameters
	UpdateInterval int64 // Model update interval (W_upd in paper), default: 100

	// Anomaly detection parameters
	FalsePositiveRate float64 // Target false positive rate for threshold, default: 0.01
	MinObservations   int     // Minimum observations before detecting anomalies, default: 5
}

// DefaultFFADEConfig returns an FFADEConfig with sensible defaults from the paper.
func DefaultFFADEConfig() FFADEConfig {
	return FFADEConfig{
		LatentDim:         10,
		LearningRate:      0.01,
		Regularization:    0.001,
		SGDIterations:     10,
		DecayRate:         0.001,
		MinFrequency:      0.01,
		UpdateInterval:    100,
		FalsePositiveRate: 0.01,
		MinObservations:   5,
	}
}

// FFADEProcessor detects anomalies in edge streams using frequency factorization.
// It implements SignalProcessor for Layer 2 correlation.
//
// Algorithm (from paper):
//  1. Maintain ITFM (decayed edge frequencies)
//  2. Periodically fit latent factors via Poisson likelihood maximization
//  3. Score edges using negative log-likelihood
//  4. Use statistically-derived threshold based on FPR
type FFADEProcessor struct {
	config FFADEConfig

	// ITFM: Interaction Time-Frequency Matrix
	itfm *ITFM

	// Matrix factorization: λ_{s,d} = W_s · H_d
	W map[string][]float64 // Source node latent factors
	H map[string][]float64 // Dest node latent factors

	// Update tracking
	flushCount      int64
	lastModelUpdate int64

	// Anomaly threshold (statistically derived)
	anomalyThreshold float64

	// Detected anomalies
	anomalousEdges map[edgeKey]*edgeAnomaly

	currentTime int64
}

// ITFM (Interaction Time-Frequency Matrix) tracks decayed edge frequencies.
type ITFM struct {
	frequencies map[edgeKey]*edgeFrequency
}

// edgeFrequency stores decayed frequency for an edge.
type edgeFrequency struct {
	freq       float64 // Current decayed frequency f_{s,d}(t)
	lastUpdate int64   // Last interaction timestamp
	totalCount int     // Total interactions observed
}

// edgeKey uniquely identifies an edge between two sources.
type edgeKey struct {
	source1 string
	source2 string
}

// edgeAnomaly represents a detected anomalous edge.
type edgeAnomaly struct {
	source1        string
	source2        string
	actualFreq     float64
	expectedFreq   float64
	deviationScore float64 // Negative log-likelihood
	timestamp      int64
}

// trainingExample represents an edge observation for model fitting.
type trainingExample struct {
	source string
	dest   string
	freq   float64
}

// NewFFADEProcessor creates a new F-FADE anomaly detector.
func NewFFADEProcessor(config FFADEConfig) *FFADEProcessor {
	return &FFADEProcessor{
		config: config,
		itfm: &ITFM{
			frequencies: make(map[edgeKey]*edgeFrequency),
		},
		W:                make(map[string][]float64),
		H:                make(map[string][]float64),
		anomalousEdges:   make(map[edgeKey]*edgeAnomaly),
		anomalyThreshold: 10.0, // Initial threshold
	}
}

// Name returns the processor name for debugging.
func (f *FFADEProcessor) Name() string {
	return "ffade"
}

// Process updates the ITFM with an edge observation.
// In Datadog context, signals represent edges if they encode interaction partners.
func (f *FFADEProcessor) Process(signal observer.Signal) {
	if signal.Timestamp > f.currentTime {
		f.currentTime = signal.Timestamp
	}

	// Extract interaction partner from signal
	// Signals should encode edges, e.g., via tags like "interacts_with:service.B"
	partner := extractInteractionPartner(signal)
	if partner == "" {
		return // Not an edge signal
	}

	// Determine edge weight (default: 1.0, or use signal score)
	weight := 1.0
	if signal.Score != nil && *signal.Score > 0 {
		weight = *signal.Score
	}

	// Update ITFM with decayed frequency
	f.itfm.Update(signal.Source, partner, signal.Timestamp, weight, f.config.DecayRate)
}

// Flush performs periodic model updates and anomaly detection.
func (f *FFADEProcessor) Flush() {
	f.flushCount++

	// Check if it's time for model update (every W_upd flushes)
	if f.flushCount%f.config.UpdateInterval == 0 {
		f.updateModel()
	}

	// Score all edges for anomalies
	f.scoreEdges()

	// Evict low-frequency edges (memory management)
	f.itfm.Evict(f.config.MinFrequency)
}

// updateModel performs periodic matrix factorization fitting.
func (f *FFADEProcessor) updateModel() {
	// 1. Build training set from ITFM
	positives := f.buildTrainingSet()
	if len(positives) < f.config.MinObservations {
		return // Not enough data
	}

	// 2. Sample negative pairs for regularization
	negatives := f.sampleNegativePairs(len(positives))

	// 3. Fit latent factors via Poisson likelihood maximization
	f.fitFactors(positives, negatives)

	// 4. Update anomaly threshold based on FPR
	f.updateThreshold()

	f.lastModelUpdate = f.currentTime
}

// buildTrainingSet constructs training examples from ITFM.
func (f *FFADEProcessor) buildTrainingSet() []trainingExample {
	examples := make([]trainingExample, 0, len(f.itfm.frequencies))

	for key, entry := range f.itfm.frequencies {
		if entry.totalCount >= f.config.MinObservations {
			examples = append(examples, trainingExample{
				source: key.source1,
				dest:   key.source2,
				freq:   entry.freq,
			})
		}
	}

	return examples
}

// sampleNegativePairs samples negative edges for model regularization.
// Negative edges are pairs that don't interact or interact rarely.
func (f *FFADEProcessor) sampleNegativePairs(numPositives int) []trainingExample {
	// Sample 50% as many negatives as positives (from paper)
	numNegatives := numPositives / 2
	if numNegatives < 1 {
		numNegatives = 1
	}

	negatives := make([]trainingExample, 0, numNegatives)
	nodes := f.getAllNodes()

	if len(nodes) < 2 {
		return negatives
	}

	attempts := 0
	maxAttempts := numNegatives * 10

	for len(negatives) < numNegatives && attempts < maxAttempts {
		attempts++

		// Sample random pair
		s := nodes[rand.Intn(len(nodes))]
		d := nodes[rand.Intn(len(nodes))]

		if s == d {
			continue // Skip self-loops
		}

		key := makeEdgeKey(s, d)

		// Check if this is a negative (no or very low frequency)
		entry := f.itfm.frequencies[key]
		if entry == nil || entry.freq < f.config.MinFrequency {
			negatives = append(negatives, trainingExample{
				source: s,
				dest:   d,
				freq:   1e-10, // Small epsilon instead of zero
			})
		}
	}

	return negatives
}

// getAllNodes returns all unique nodes from ITFM.
func (f *FFADEProcessor) getAllNodes() []string {
	nodeSet := make(map[string]struct{})

	for key := range f.itfm.frequencies {
		nodeSet[key.source1] = struct{}{}
		nodeSet[key.source2] = struct{}{}
	}

	nodes := make([]string, 0, len(nodeSet))
	for node := range nodeSet {
		nodes = append(nodes, node)
	}

	return nodes
}

// fitFactors fits latent factors to maximize Poisson likelihood.
// Model: λ_{s,d} = W_s · H_d
// Loss: -Σ log P(freq_{s,d} | λ_{s,d}) + regularization
func (f *FFADEProcessor) fitFactors(positives, negatives []trainingExample) {
	allExamples := append(positives, negatives...)

	// Initialize factors for new nodes
	for _, ex := range allExamples {
		f.ensureFactorExists(ex.source)
		f.ensureFactorExists(ex.dest)
	}

	// SGD iterations
	for iter := 0; iter < f.config.SGDIterations; iter++ {
		// Shuffle training examples
		rand.Shuffle(len(allExamples), func(i, j int) {
			allExamples[i], allExamples[j] = allExamples[j], allExamples[i]
		})

		for _, ex := range allExamples {
			f.sgdUpdate(ex.source, ex.dest, ex.freq)
		}
	}
}

// sgdUpdate performs SGD update for a single edge.
func (f *FFADEProcessor) sgdUpdate(s, d string, observedFreq float64) {
	Ws := f.W[s]
	Hd := f.H[d]

	// Compute predicted frequency: λ = W_s · H_d
	lambda := dotProduct(Ws, Hd)
	if lambda < 1e-10 {
		lambda = 1e-10
	}

	// Gradient of negative log-Poisson likelihood
	// ∂(-log P)/∂λ = 1 - (observed / λ)
	gradLambda := 1.0 - observedFreq/lambda

	// Backprop through dot product and add L2 regularization
	for i := 0; i < f.config.LatentDim; i++ {
		gradW := gradLambda*Hd[i] + f.config.Regularization*Ws[i]
		gradH := gradLambda*Ws[i] + f.config.Regularization*Hd[i]

		// Update with learning rate
		Ws[i] -= f.config.LearningRate * gradW
		Hd[i] -= f.config.LearningRate * gradH
	}
}

// ensureFactorExists initializes latent factors for a new node.
func (f *FFADEProcessor) ensureFactorExists(node string) {
	if _, exists := f.W[node]; !exists {
		f.W[node] = randomVector(f.config.LatentDim)
	}
	if _, exists := f.H[node]; !exists {
		f.H[node] = randomVector(f.config.LatentDim)
	}
}

// updateThreshold derives statistical threshold based on FPR.
func (f *FFADEProcessor) updateThreshold() {
	// Collect scores for all current edges
	scores := make([]float64, 0, len(f.itfm.frequencies))

	for key, entry := range f.itfm.frequencies {
		if entry.totalCount < f.config.MinObservations {
			continue
		}

		lambda := f.computeLambda(key.source1, key.source2)
		score := f.poissonAnomalyScore(entry.freq, lambda)
		scores = append(scores, score)
	}

	if len(scores) == 0 {
		return
	}

	// Sort scores
	sort.Float64s(scores)

	// Threshold at (1 - FPR) quantile
	quantileIdx := int(float64(len(scores)) * (1.0 - f.config.FalsePositiveRate))
	if quantileIdx >= len(scores) {
		quantileIdx = len(scores) - 1
	}

	f.anomalyThreshold = scores[quantileIdx]
}

// scoreEdges scores all edges and flags anomalies.
func (f *FFADEProcessor) scoreEdges() {
	f.anomalousEdges = make(map[edgeKey]*edgeAnomaly)

	for key, entry := range f.itfm.frequencies {
		if entry.totalCount < f.config.MinObservations {
			continue
		}

		// Compute predicted intensity
		lambda := f.computeLambda(key.source1, key.source2)

		// Compute anomaly score
		score := f.poissonAnomalyScore(entry.freq, lambda)

		// Flag if above threshold
		if score > f.anomalyThreshold {
			f.anomalousEdges[key] = &edgeAnomaly{
				source1:        key.source1,
				source2:        key.source2,
				actualFreq:     entry.freq,
				expectedFreq:   lambda,
				deviationScore: score,
				timestamp:      f.currentTime,
			}
		}
	}
}

// computeLambda computes predicted edge frequency: λ_{s,d} = W_s · H_d
func (f *FFADEProcessor) computeLambda(s, d string) float64 {
	// Ensure factors exist
	f.ensureFactorExists(s)
	f.ensureFactorExists(d)

	return math.Max(dotProduct(f.W[s], f.H[d]), 1e-10)
}

// poissonAnomalyScore computes negative log-likelihood score.
// Higher score = more anomalous
func (f *FFADEProcessor) poissonAnomalyScore(observed, lambda float64) float64 {
	if lambda < 1e-10 {
		lambda = 1e-10
	}

	// -log P(n|λ) = λ - n*log(λ) + log(n!)
	// Use Stirling approximation: log(n!) ≈ n*log(n) - n
	score := lambda - observed*math.Log(lambda)
	if observed > 1.0 {
		score += observed*math.Log(observed) - observed
	}

	return score
}

// AnomalousInteractions returns currently detected anomalous edges.
func (f *FFADEProcessor) AnomalousInteractions() []EdgeAnomaly {
	result := make([]EdgeAnomaly, 0, len(f.anomalousEdges))

	for _, anomaly := range f.anomalousEdges {
		result = append(result, EdgeAnomaly{
			Source1:        anomaly.source1,
			Source2:        anomaly.source2,
			ActualFreq:     anomaly.actualFreq,
			ExpectedFreq:   anomaly.expectedFreq,
			DeviationScore: anomaly.deviationScore,
			Timestamp:      anomaly.timestamp,
		})
	}

	// Sort by deviation score (most anomalous first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].DeviationScore > result[j].DeviationScore
	})

	return result
}

// EdgeAnomaly represents an anomalous edge interaction.
type EdgeAnomaly struct {
	Source1        string
	Source2        string
	ActualFreq     float64
	ExpectedFreq   float64
	DeviationScore float64
	Timestamp      int64
}

// Update updates ITFM with decayed frequency for an edge.
func (itfm *ITFM) Update(s, d string, t int64, weight, decayRate float64) {
	key := makeEdgeKey(s, d)
	entry := itfm.frequencies[key]

	if entry == nil {
		entry = &edgeFrequency{}
		itfm.frequencies[key] = entry
	}

	// Apply temporal decay since last update
	timeDelta := t - entry.lastUpdate
	if timeDelta > 0 && entry.totalCount > 0 {
		entry.freq *= math.Exp(-decayRate * float64(timeDelta))
	}

	// Add new observation
	entry.freq += weight
	entry.lastUpdate = t
	entry.totalCount++
}

// Evict removes low-frequency edges.
func (itfm *ITFM) Evict(minFreq float64) {
	for key, entry := range itfm.frequencies {
		if entry.freq < minFreq {
			delete(itfm.frequencies, key)
		}
	}
}

// makeEdgeKey creates a canonical edge key (lexicographic ordering).
func makeEdgeKey(s, d string) edgeKey {
	if s < d {
		return edgeKey{source1: s, source2: d}
	}
	return edgeKey{source1: d, source2: s}
}

// String returns string representation of edge key.
func (e edgeKey) String() string {
	return fmt.Sprintf("%s <-> %s", e.source1, e.source2)
}

// extractInteractionPartner extracts the interaction partner from signal tags.
// Looks for tags like "interacts_with:service.B" or "edge_dest:service.B"
func extractInteractionPartner(signal observer.Signal) string {
	for _, tag := range signal.Tags {
		// Check for interaction tags
		if len(tag) > 15 && tag[:15] == "interacts_with:" {
			return tag[15:]
		}
		if len(tag) > 10 && tag[:10] == "edge_dest:" {
			return tag[10:]
		}
	}
	return ""
}

// randomVector creates a random latent factor vector with small values.
func randomVector(dim int) []float64 {
	vec := make([]float64, dim)
	for i := range vec {
		vec[i] = (rand.Float64() - 0.5) * 0.1 // Small random values [-0.05, 0.05]
	}
	return vec
}

// dotProduct computes dot product of two vectors.
func dotProduct(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}

	result := 0.0
	for i := range a {
		result += a[i] * b[i]
	}
	return result
}
