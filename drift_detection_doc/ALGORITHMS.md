# Algorithm Details

## 1. Shannon Entropy for Template Extraction

### Mathematical Definition

Shannon entropy measures the uncertainty/variability in a random variable:

```
H(X) = -Σ p(xᵢ) log₂(p(xᵢ))
```

Where:
- `X` = set of tokens at a specific position
- `p(xᵢ)` = probability of token `xᵢ` = `count(xᵢ) / total`

### Interpretation

- **H = 0**: All tokens identical (constant field)
- **H = 1**: Binary variation (e.g., true/false)
- **H > 2.5**: High variability (likely a variable field)

### Example

Position 3 across 100 logs:
```
Tokens: ["INFO", "INFO", "WARN", "INFO", "INFO", ...]
Counts: INFO=90, WARN=10
p(INFO) = 0.9, p(WARN) = 0.1
H = -(0.9×log₂(0.9) + 0.1×log₂(0.1))
H = -(0.9×-0.152 + 0.1×-3.322)
H = 0.137 + 0.332 = 0.469

Decision: H < 2.5 → Keep as constant (or detect as enum)
```

### Enum Detection Heuristic

Enums are high-entropy but low-cardinality:
```
if H > 2.5 AND |unique_values| < 10:
    is_enum = true  # Keep original value
else if H > 2.5:
    is_variable = true  # Replace with <*>
else:
    is_constant = true  # Keep original value
```

### Implementation Complexity

```go
func CalculateEntropy(tokens []string) float64 {
    counts := make(map[string]int)
    for _, tok := range tokens {
        counts[tok]++  // O(n)
    }

    total := float64(len(tokens))
    entropy := 0.0
    for _, count := range counts {  // O(k) where k = unique tokens
        p := float64(count) / total
        entropy -= p * math.Log2(p)
    }
    return entropy
}

// Overall: O(n + k) ≈ O(n) where n = len(tokens)
```

## 2. Hankel DMD (Dynamic Mode Decomposition)

### Problem Statement

Given a time series of high-dimensional observations (embeddings):
```
X = [x₁ x₂ x₃ ... xₙ]  where xᵢ ∈ ℝᵈ (d=768)
```

Learn a linear dynamical system that predicts:
```
xₖ₊₁ = A xₖ
```

Where `A` is the system matrix representing dynamics.

### Hankel Extension (Time-Delay Embedding)

Standard DMD assumes Markov property (next state depends only on current).
Hankel DMD captures longer dependencies using time delays.

**Time-delay state augmentation** (d=5):
```
yₖ = [xₖ, xₖ₊₁, xₖ₊₂, xₖ₊₃, xₖ₊₄]ᵀ  ∈ ℝ³⁸⁴⁰
```

**Hankel matrix construction**:
```
H = [y₁ y₂ y₃ ... yₙ₋₄]
  = [x₁  x₂  x₃  ... xₙ₋₄]   ← t
    [x₂  x₃  x₄  ... xₙ₋₃]   ← t+1
    [x₃  x₄  x₅  ... xₙ₋₂]   ← t+2
    [x₄  x₅  x₆  ... xₙ₋₁]   ← t+3
    [x₅  x₆  x₇  ... xₙ  ]   ← t+4

Shape: (768×5, n-4) = (3840, n-4)
```

### DMD Algorithm Steps

1. **Split data**: `H₁ = H[:, :-1]`, `H₂ = H[:, 1:]`
   ```
   H₁ = [y₁ y₂ ... yₙ₋₅]
   H₂ = [y₂ y₃ ... yₙ₋₄]
   ```

2. **Compute best-fit linear operator**: `H₂ ≈ A H₁`
   ```
   A = H₂ H₁⁺  (where H₁⁺ is pseudoinverse)
   ```

3. **SVD of H₁** (dimensionality reduction):
   ```
   H₁ = U Σ Vᵀ

   U: Left singular vectors (spatial modes)
   Σ: Singular values (energy)
   V: Right singular vectors (time coefficients)
   ```

4. **Reduced operator** (rank-r approximation):
   ```
   Ã = Uᵣᵀ A Uᵣ = Uᵣᵀ H₂ Vᵣ Σᵣ⁻¹

   Where Uᵣ, Σᵣ, Vᵣ are top-r components
   ```

5. **Eigendecomposition** of Ã:
   ```
   Ã W = W Λ

   W: Eigenvectors
   Λ: Eigenvalues (diagonal)
   ```

6. **DMD modes** (full-dimensional):
   ```
   Φ = H₂ Vᵣ Σᵣ⁻¹ W
   ```

7. **Reconstruction**:
   ```
   X_reconstructed = Φ Λᵏ b

   Where:
   - Φ: DMD modes
   - Λᵏ: Eigenvalues raised to power k (time evolution)
   - b: Initial amplitudes (from x₁)
   ```

### Reconstruction Error

For each time step k:
```
error[k] = ||X[:, k] - X_reconstructed[:, k]||₂
         = √(Σᵢ (xᵢₖ - x̂ᵢₖ)²)
```

Normalized error:
```
normalized_error[k] = (error[k] - μ_error) / σ_error
```

### Eigenvalue Interpretation

Each eigenvalue λⱼ ∈ ℂ characterizes a mode:

```
λⱼ = rⱼ e^(iωⱼ)

Where:
- rⱼ = |λⱼ|: Growth rate
  - rⱼ < 1: Decaying mode (stable)
  - rⱼ = 1: Constant amplitude (neutral)
  - rⱼ > 1: Growing mode (unstable)

- ωⱼ = arg(λⱼ): Angular frequency
  - ωⱼ = 0: No oscillation
  - ωⱼ ≠ 0: Oscillatory behavior
```

Growth rate in log scale:
```
growth_rate = ln(|λⱼ|)
```

### Why Hankel DMD Detects Anomalies

1. **Training**: DMD learns normal dynamics from historical embeddings
2. **Prediction**: Uses learned modes/eigenvalues to reconstruct current state
3. **Anomaly**: When actual state deviates from prediction:
   ```
   error = ||actual - predicted||₂ → large
   ```
4. **Threshold**: Statistical outliers (>2σ or >3σ) flagged as anomalies

**Key Insight**: System behaving differently than learned patterns = anomaly

### Implementation Notes

**Packages needed**:
- `gonum.org/v1/gonum/mat`: Dense matrix operations
- `gonum.org/v1/gonum/lapack`: SVD, eigenvalue decomposition

**Optimization**:
- Cache SVD results when possible
- Recompute only every N windows (e.g., 10)
- Use sparse matrices if applicable

**Complexity**:
- SVD: O(min(m²n, mn²)) for m×n matrix
- For (3840, 120): ~O(120² × 3840) = O(55M) operations
- Modern CPU: ~50ms per computation
