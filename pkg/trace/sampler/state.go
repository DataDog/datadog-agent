package sampler

// InternalState exposes all the main internal settings of the score sampler
type InternalState struct {
	Offset      float64
	Slope       float64
	Cardinality int64
	InTPS       float64
	OutTPS      float64
	MaxTPS      float64
}

// GetState collects and return internal statistics and coefficients for indication purposes
func (s *Sampler) GetState() InternalState {
	return InternalState{
		Offset:      s.signatureScoreOffset.Load(),
		Slope:       s.signatureScoreSlope.Load(),
		Cardinality: s.Backend.GetCardinality(),
		InTPS:       s.Backend.GetTotalScore(),
		OutTPS:      s.Backend.GetSampledScore(),
		MaxTPS:      s.maxTPS,
	}
}
