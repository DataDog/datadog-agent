// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dmd

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gonum.org/v1/gonum/mat"
)

// Analyzer performs Online DMD analysis using Recursive Least Squares
type Analyzer struct {
	sourceKey  string
	config     common.DMDConfig
	inputChan  chan common.EmbeddingResult
	outputChan chan common.DMDResult
	ctx        context.Context
	cancel     context.CancelFunc

	mu sync.RWMutex

	// Online DMD state
	hankelBuffer *RingBuffer // Circular buffer of last d vectors
	A            *mat.Dense  // System matrix (incrementally updated via RLS)
	P            *mat.Dense  // Covariance matrix for RLS updates
	initialized  bool        // Whether DMD has been initialized

	// Running statistics for error normalization
	errorHistory []float64 // Sliding window of recent errors
	errorSum     float64   // Running sum for mean calculation
	errorSumSq   float64   // Running sum of squares for std calculation
	dim          int       // Embedding dimension (e.g., 768 for embeddinggemma)
}

// RingBuffer implements a circular buffer for efficient Hankel matrix management
type RingBuffer struct {
	vectors []common.Vector // Pre-allocated vector storage
	size    int             // Buffer capacity (TimeDelay)
	head    int             // Write position
	count   int             // Number of vectors currently stored
}

// NewRingBuffer creates a ring buffer with specified capacity and vector dimension
func NewRingBuffer(size, dim int) *RingBuffer {
	vectors := make([]common.Vector, size)
	for i := range vectors {
		vectors[i] = make(common.Vector, dim)
	}
	return &RingBuffer{
		vectors: vectors,
		size:    size,
		head:    0,
		count:   0,
	}
}

// Add inserts a new vector into the ring buffer, overwriting the oldest if full
func (rb *RingBuffer) Add(vec common.Vector) {
	copy(rb.vectors[rb.head], vec)
	rb.head = (rb.head + 1) % rb.size
	if rb.count < rb.size {
		rb.count++
	}
}

// IsFull returns true if the buffer contains d vectors
func (rb *RingBuffer) IsFull() bool {
	return rb.count == rb.size
}

// GetOldest returns the oldest vector in the buffer (second-to-last for prediction)
func (rb *RingBuffer) GetOldest() common.Vector {
	if rb.count < 2 {
		return nil
	}
	// Get second-to-last vector
	idx := (rb.head - 2 + rb.size) % rb.size
	return rb.vectors[idx]
}

// GetNewest returns the most recent vector in the buffer
func (rb *RingBuffer) GetNewest() common.Vector {
	if rb.count == 0 {
		return nil
	}
	idx := (rb.head - 1 + rb.size) % rb.size
	return rb.vectors[idx]
}

// GetAllVectors returns all vectors in chronological order
func (rb *RingBuffer) GetAllVectors() []common.Vector {
	result := make([]common.Vector, rb.count)
	for i := 0; i < rb.count; i++ {
		idx := (rb.head - rb.count + i + rb.size) % rb.size
		result[i] = rb.vectors[idx]
	}
	return result
}

// NewAnalyzer creates a new Online DMD analyzer for a specific source
func NewAnalyzer(sourceKey string, config common.DMDConfig, inputChan chan common.EmbeddingResult, outputChan chan common.DMDResult) *Analyzer {
	ctx, cancel := context.WithCancel(context.Background())
	return &Analyzer{
		sourceKey:    sourceKey,
		config:       config,
		inputChan:    inputChan,
		outputChan:   outputChan,
		ctx:          ctx,
		cancel:       cancel,
		errorHistory: make([]float64, 0, config.ErrorHistory),
	}
}

// Start begins processing embedding results
func (a *Analyzer) Start() {
	go a.run()
}

// Stop stops the analyzer
func (a *Analyzer) Stop() {
	a.cancel()
}

func (a *Analyzer) run() {
	for {
		select {
		case <-a.ctx.Done():
			close(a.outputChan)
			return

		case result, ok := <-a.inputChan:
			if !ok {
				close(a.outputChan)
				return
			}

			// Process the embedding result with Online DMD
			dmdResult := a.processEmbeddingOnline(result)
			if dmdResult != nil {
				a.outputChan <- *dmdResult
			}
		}
	}
}

func (a *Analyzer) processEmbeddingOnline(result common.EmbeddingResult) *common.DMDResult {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Average embeddings to get a single representative vector
	// If no embeddings (empty window), create a zero vector as placeholder
	avgVector := averageVectors(result.Embeddings)
	if avgVector == nil {
		// Initialize dimension from first non-empty window if possible
		if a.dim == 0 {
			a.dim = 768 // Default dimension for embeddinggemma model
		}
		avgVector = make(common.Vector, a.dim)
		// Leave as zeros - represents no activity in this window
	} else if a.dim == 0 {
		// First non-empty window - initialize dimension
		a.dim = len(avgVector)
	}

	// Initialize ring buffer on first vector
	if a.hankelBuffer == nil {
		a.hankelBuffer = NewRingBuffer(a.config.TimeDelay, a.dim)
	}

	// Add new vector to Hankel buffer
	a.hankelBuffer.Add(avgVector)

	// Need at least d vectors before we can initialize DMD
	if !a.hankelBuffer.IsFull() {
		return nil
	}

	// Initialize DMD on first full buffer
	if !a.initialized {
		err := a.initializeOnlineDMD()
		if err != nil {
			log.Warnf("Failed to initialize Online DMD for source %s: %v", a.sourceKey, err)
			return nil
		}
		log.Infof("Online DMD initialized for source %s (buffer filled with %d vectors)", a.sourceKey, a.config.TimeDelay)
		a.initialized = true
	}

	// Update DMD model incrementally with RLS
	err := a.updateOnlineDMD()
	if err != nil {
		log.Warnf("Failed to update Online DMD for source %s: %v", a.sourceKey, err)
		return nil
	}

	// Compute reconstruction error for this window
	reconstructionError := a.computeReconstructionError()

	// Update running statistics
	a.updateErrorStatistics(reconstructionError)

	// Normalize error (z-score)
	normalizedError := 0.0
	if len(a.errorHistory) > 1 {
		mean := a.errorSum / float64(len(a.errorHistory))
		variance := (a.errorSumSq / float64(len(a.errorHistory))) - (mean * mean)
		if variance > 0 {
			std := math.Sqrt(variance)
			normalizedError = (reconstructionError - mean) / std
		}
	}

	return &common.DMDResult{
		SourceKey:           a.sourceKey,
		WindowID:            result.WindowID,
		ReconstructionError: reconstructionError,
		NormalizedError:     normalizedError,
		Templates:           result.Templates,
	}
}

// initializeOnlineDMD performs one-time initialization when buffer first fills
func (a *Analyzer) initializeOnlineDMD() error {
	vectors := a.hankelBuffer.GetAllVectors()
	if len(vectors) < 2 {
		return fmt.Errorf("need at least 2 vectors for initialization, have %d", len(vectors))
	}

	// Build X and Y matrices for initial fit: X = [v1, v2, ..., v(d-1)], Y = [v2, v3, ..., vd]
	d := len(vectors)
	dim := len(vectors[0])

	// X matrix: all vectors except the last
	xData := make([]float64, dim*(d-1))
	for col := 0; col < d-1; col++ {
		for row := 0; row < dim; row++ {
			xData[row*(d-1)+col] = vectors[col][row]
		}
	}
	X := mat.NewDense(dim, d-1, xData)

	// Y matrix: all vectors except the first
	yData := make([]float64, dim*(d-1))
	for col := 0; col < d-1; col++ {
		for row := 0; row < dim; row++ {
			yData[row*(d-1)+col] = vectors[col+1][row]
		}
	}
	Y := mat.NewDense(dim, d-1, yData)

	// Solve for initial A: A = Y * X^+ (pseudoinverse)
	// For X with dim×(d-1), compute X^+ with (d-1)×dim
	var svd mat.SVD
	if !svd.Factorize(X.T(), mat.SVDThin) {
		return fmt.Errorf("SVD factorization failed during initialization")
	}

	// When we factorize X.T() (which is (d-1)×dim), we get:
	// X.T() = U * S * V^T
	// So X.T()^+ = V * S^-1 * U^T
	// And X^+ = (X.T()^+)^T = U * S^-1 * V^T
	var U, V mat.Dense
	svd.UTo(&U)
	svd.VTo(&V)
	s := svd.Values(nil)

	// Create S^-1 (invert singular values, filter small ones for numerical stability)
	sInv := make([]float64, len(s))
	for i, val := range s {
		if val > 1e-10 {
			sInv[i] = 1.0 / val
		}
	}
	SInv := mat.NewDiagDense(len(s), sInv)

	// X^+ = U * S^-1 * V^T (transpose of X.T()^+)
	var temp mat.Dense
	temp.Mul(&U, SInv)
	var XPinv mat.Dense
	XPinv.Mul(&temp, V.T())

	// A = Y * X^+ with dims: (dim×(d-1)) * ((d-1)×dim) = (dim×dim)
	a.A = mat.NewDense(dim, dim, nil)
	a.A.Mul(Y, &XPinv)

	// Initialize covariance matrix P = (X * X^T + λI)^-1
	var XXT mat.Dense
	XXT.Mul(X, X.T())

	// Add regularization: P = (X*X^T + λI)^-1
	lambda := 1e-6 // Small regularization for numerical stability
	for i := 0; i < dim; i++ {
		XXT.Set(i, i, XXT.At(i, i)+lambda)
	}

	// Invert to get P
	a.P = mat.NewDense(dim, dim, nil)
	err := a.P.Inverse(&XXT)
	if err != nil {
		return fmt.Errorf("failed to invert covariance matrix: %v", err)
	}

	return nil
}

// updateOnlineDMD performs incremental RLS update of system matrix A
func (a *Analyzer) updateOnlineDMD() error {
	// Get x (second-to-last) and y (latest) vectors
	x := a.hankelBuffer.GetOldest()
	y := a.hankelBuffer.GetNewest()

	if x == nil || y == nil {
		return fmt.Errorf("insufficient vectors in buffer")
	}

	dim := len(x)

	// Convert vectors to column matrices
	xMat := mat.NewDense(dim, 1, x)
	yMat := mat.NewDense(dim, 1, y)

	// Compute Kalman gain: K = P * x / (λ + x^T * P * x)
	lambda := a.config.RLSLambda

	// Compute P * x
	var Px mat.Dense
	Px.Mul(a.P, xMat)

	// Compute x^T * P * x (scalar)
	var xTPx mat.Dense
	xTPx.Mul(xMat.T(), &Px)
	denominator := lambda + xTPx.At(0, 0)

	// K = (P * x) / denominator
	K := mat.NewDense(dim, 1, nil)
	K.Scale(1.0/denominator, &Px)

	// Compute innovation: innovation = y - A * x
	var Ax mat.Dense
	Ax.Mul(a.A, xMat)

	innovation := mat.NewDense(dim, 1, nil)
	innovation.Sub(yMat, &Ax)

	// Update A: A = A + innovation * K^T
	var update mat.Dense
	update.Mul(innovation, K.T())
	a.A.Add(a.A, &update)

	// Update P: P = (P - K * x^T * P) / λ
	var KxTP mat.Dense
	KxTP.Mul(K, xMat.T())

	var temp mat.Dense
	temp.Mul(&KxTP, a.P)

	a.P.Sub(a.P, &temp)
	a.P.Scale(1.0/lambda, a.P)

	return nil
}

// computeReconstructionError calculates L2 norm of prediction error
func (a *Analyzer) computeReconstructionError() float64 {
	x := a.hankelBuffer.GetOldest()
	y := a.hankelBuffer.GetNewest()

	if x == nil || y == nil {
		return 0.0
	}

	dim := len(x)

	// Predict: y_pred = A * x
	xMat := mat.NewDense(dim, 1, x)
	yPred := mat.NewDense(dim, 1, nil)
	yPred.Mul(a.A, xMat)

	// Compute error: ||y - y_pred||_2
	errorSum := 0.0
	for i := 0; i < dim; i++ {
		diff := y[i] - yPred.At(i, 0)
		errorSum += diff * diff
	}

	return math.Sqrt(errorSum)
}

// updateErrorStatistics maintains sliding window of errors for normalization
func (a *Analyzer) updateErrorStatistics(newError float64) {
	// Add new error
	a.errorHistory = append(a.errorHistory, newError)
	a.errorSum += newError
	a.errorSumSq += newError * newError

	// Remove oldest if exceeding history size
	if len(a.errorHistory) > a.config.ErrorHistory {
		oldestError := a.errorHistory[0]
		a.errorHistory = a.errorHistory[1:]
		a.errorSum -= oldestError
		a.errorSumSq -= oldestError * oldestError
	}
}

// averageVectors computes the element-wise average of a list of vectors
func averageVectors(vectors []common.Vector) common.Vector {
	if len(vectors) == 0 {
		return nil
	}

	dim := len(vectors[0])
	avg := make(common.Vector, dim)

	for _, vec := range vectors {
		for i, val := range vec {
			avg[i] += val
		}
	}

	for i := range avg {
		avg[i] /= float64(len(vectors))
	}

	return avg
}
