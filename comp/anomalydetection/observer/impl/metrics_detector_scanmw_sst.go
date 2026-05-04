// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import "math"

// maxHankelL is the maximum supported Hankel matrix row count. Capping L at
// this value keeps C = H Hᵀ on the stack (fixed [maxHankelL*maxHankelL]float64).
const maxHankelL = 16

// sstScore computes 1 − |⟨u_pre, u_post⟩| where u_* are the leading left
// singular vectors of the Hankel trajectory matrices built from pre and post.
//
// Return value semantics:
//   - 0.0 sentinel: window too short or invalid L (caller should not have called)
//   - 1.0 fail-open: window is constant (zero temporal structure) or degenerate;
//     the candidate is passed through unchanged — we cannot make a judgement
//   - (0, 1): genuine SST score; values ≥ SSTMinScore keep the candidate
//
// Constant signals (all identical values) always produce the same uniform
// leading singular vector regardless of level, so |dot|=1 and score=0 would
// incorrectly suppress every level shift in flat series. We detect this via
// the variance gate and return 1.0 (fail-open) instead.
//
// Time complexity: O(L² K) per side where K = len(side) − L + 1.
// For the default L=8, K≈17 that is ~2500 multiplications per side.
func sstScore(pre, post []float64, L, powerIters int) float64 {
	if L <= 0 || L > maxHankelL {
		return 0
	}
	// Sentinel 0: windows too short for a meaningful Hankel (K < 2 needs L+1
	// points minimum; we require L+2 to give K ≥ 2).
	if len(pre) < L+2 || len(post) < L+2 {
		return 0
	}
	// Fail-open for constant windows: a purely flat signal embeds to an
	// all-identical-columns Hankel, giving the same uniform leading singular
	// vector at any level. Comparing pre/post would always score 0 and
	// suppress every level shift. Skip SST and let the candidate through.
	if windowVariance(pre) < 1e-20 || windowVariance(post) < 1e-20 {
		return 1.0
	}

	uPre, ok := leadingSingularVector(pre, L, powerIters)
	if !ok {
		return 1.0 // degenerate: fail-open
	}
	uPost, ok := leadingSingularVector(post, L, powerIters)
	if !ok {
		return 1.0 // degenerate: fail-open
	}

	// Dot product of the two leading left singular vectors.
	dot := 0.0
	for i := 0; i < L; i++ {
		dot += uPre[i] * uPost[i]
	}
	score := 1.0 - math.Abs(dot)

	// Numerical guards: NaN/Inf mean the computation blew up — fail-open.
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 1.0
	}
	if score < 0 {
		score = 0
	}
	return score
}

// windowVariance computes the population variance of data.
// Returns 0 for empty or single-element slices.
func windowVariance(data []float64) float64 {
	n := len(data)
	if n < 2 {
		return 0
	}
	sum := 0.0
	for _, v := range data {
		sum += v
	}
	mean := sum / float64(n)
	v2 := 0.0
	for _, v := range data {
		d := v - mean
		v2 += d * d
	}
	return v2 / float64(n)
}

// leadingSingularVector returns the leading left singular vector of the
// Hankel trajectory matrix built from data, using power iteration on H Hᵀ.
// Returns (vector, true) on success or (zero, false) when data is too short
// or the trajectory is numerically degenerate (caller should fail-open).
func leadingSingularVector(data []float64, L, powerIters int) ([maxHankelL]float64, bool) {
	var zero [maxHankelL]float64

	K := len(data) - L + 1
	if K < 2 {
		return zero, false
	}

	// Build C = H Hᵀ as an L×L matrix stored in a fixed-size stack array.
	// H[i,j] = data[i+j] so C[i,k] = Σ_j data[i+j]*data[k+j].
	var C [maxHankelL * maxHankelL]float64
	for i := 0; i < L; i++ {
		for k := i; k < L; k++ {
			sum := 0.0
			for j := 0; j < K; j++ {
				sum += data[i+j] * data[k+j]
			}
			C[i*maxHankelL+k] = sum
			C[k*maxHankelL+i] = sum // symmetric
		}
	}

	// Power iteration: v ← C v; v ← v / ||v||.
	// Initialise v = [1/sqrt(L), ...] for deterministic output.
	var v [maxHankelL]float64
	invSqrtL := 1.0 / math.Sqrt(float64(L))
	for i := 0; i < L; i++ {
		v[i] = invSqrtL
	}

	for range powerIters {
		var next [maxHankelL]float64
		for i := 0; i < L; i++ {
			s := 0.0
			for k := 0; k < L; k++ {
				s += C[i*maxHankelL+k] * v[k]
			}
			next[i] = s
		}
		// Normalise.
		norm := 0.0
		for i := 0; i < L; i++ {
			norm += next[i] * next[i]
		}
		if norm < 1e-30 {
			// All-zero trajectory after projection: degenerate.
			return zero, false
		}
		invNorm := 1.0 / math.Sqrt(norm)
		for i := 0; i < L; i++ {
			v[i] = next[i] * invNorm
		}
	}

	return v, true
}
