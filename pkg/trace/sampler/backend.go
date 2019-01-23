package sampler

// Backend stores and counts traces and signatures ingested by a sampler.
type Backend interface {
	// Run runs the blocking execution of the backend main loop.
	Run()

	// Stop stops the backend main loop.
	Stop()

	// CountSample counts that 1 trace is going through the sampler.
	CountSample()

	// CountSignature counts that 1 signature is going through the sampler.
	CountSignature(signature Signature)

	// GetTotalScore returns the TPS (Traces Per Second) of all traces ingested.
	GetTotalScore() float64

	// GetSampledScore returns the TPS of all traces sampled.
	GetSampledScore() float64

	// GetUpperSampledScore is similar to GetSampledScore, but with the upper approximation.
	GetUpperSampledScore() float64

	// GetSignatureScore returns the TPS of traces ingested of a given signature.
	GetSignatureScore(signature Signature) float64

	// GetSignatureScores returns the TPS of traces ingested for all signatures.
	GetSignatureScores() map[Signature]float64

	// GetCardinality returns the number of different signatures seen.
	GetCardinality() int64
}
