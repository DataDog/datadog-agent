// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"fmt"
	"math"
	"math/big"
	"strings"
)

// Quantity is a simplified local replacement for k8s.io/apimachinery/pkg/api/resource.Quantity.
// It parses Kubernetes resource quantity strings (e.g., "100m", "128Mi", "1.5")
// and provides numeric accessors.
//
// Supported formats:
//   - Plain integers: "1024"
//   - Decimals: "1.5"
//   - Decimal SI suffixes: n, u, m, k, M, G, T, P, E (base-10)
//   - Binary SI suffixes: Ki, Mi, Gi, Ti, Pi, Ei (base-2)
//   - Exponent notation: "1e3", "1E3"
type Quantity struct {
	raw string

	// Internal representation as a scaled integer.
	// The value is unscaled * 10^(-scale).
	// For example, "100m" is stored as unscaled=100, scale=3 (100 * 10^-3 = 0.1).
	unscaled *big.Int
	scale    int32 // number of decimal places (positive = fractional)
}

// UnmarshalJSON implements json.Unmarshaler for Quantity.
func (q *Quantity) UnmarshalJSON(data []byte) error {
	// Strip quotes
	s := string(data)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	if s == "null" || s == "" {
		return nil
	}
	parsed, err := parseQuantity(s)
	if err != nil {
		return err
	}
	*q = parsed
	return nil
}

// MarshalJSON implements json.Marshaler for Quantity.
func (q Quantity) MarshalJSON() ([]byte, error) {
	return []byte(`"` + q.String() + `"`), nil
}

// String returns the original string representation.
func (q Quantity) String() string {
	return q.raw
}

// AsApproximateFloat64 returns the quantity as a float64.
// This may lose precision for very large values.
func (q Quantity) AsApproximateFloat64() float64 {
	if q.unscaled == nil {
		return 0
	}
	f := new(big.Float).SetInt(q.unscaled)
	if q.scale > 0 {
		divisor := new(big.Float).SetFloat64(math.Pow10(int(q.scale)))
		f.Quo(f, divisor)
	} else if q.scale < 0 {
		multiplier := new(big.Float).SetFloat64(math.Pow10(int(-q.scale)))
		f.Mul(f, multiplier)
	}
	result, _ := f.Float64()
	return result
}

// Value returns the quantity value scaled to whole units (e.g., bytes for memory).
// Fractional parts are truncated.
func (q Quantity) Value() int64 {
	if q.unscaled == nil {
		return 0
	}
	if q.scale <= 0 {
		// No fractional part; multiply by 10^(-scale)
		result := new(big.Int).Set(q.unscaled)
		if q.scale < 0 {
			multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-q.scale)), nil)
			result.Mul(result, multiplier)
		}
		return result.Int64()
	}
	// Has fractional part; divide by 10^scale (truncating)
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(q.scale)), nil)
	result := new(big.Int).Div(q.unscaled, divisor)
	return result.Int64()
}

// MilliValue returns the quantity scaled to milli-units (1/1000th of a unit).
// For CPU, 1 core = 1000 millicores.
func (q Quantity) MilliValue() int64 {
	if q.unscaled == nil {
		return 0
	}
	// Multiply unscaled by 1000, then apply scale
	millis := new(big.Int).Mul(q.unscaled, big.NewInt(1000))
	if q.scale <= 0 {
		if q.scale < 0 {
			multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-q.scale)), nil)
			millis.Mul(millis, multiplier)
		}
		return millis.Int64()
	}
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(q.scale)), nil)
	millis.Div(millis, divisor)
	return millis.Int64()
}

// parseQuantity parses a Kubernetes quantity string.
func parseQuantity(s string) (Quantity, error) {
	if s == "" {
		return Quantity{}, fmt.Errorf("empty quantity string")
	}

	raw := s

	// Check for binary SI suffix first (Ki, Mi, Gi, Ti, Pi, Ei)
	if binaryBase, suffix, ok := parseBinarySuffix(s); ok {
		numStr := s[:len(s)-len(suffix)]
		unscaled, scale, err := parseDecimalString(numStr)
		if err != nil {
			return Quantity{}, fmt.Errorf("failed to parse quantity %q: %w", s, err)
		}
		// Convert to bytes: value * binaryBase
		// First get the actual numeric value: unscaled * 10^(-scale)
		// Then multiply by binaryBase
		f := new(big.Float).SetInt(unscaled)
		if scale > 0 {
			divisor := new(big.Float).SetFloat64(math.Pow10(int(scale)))
			f.Quo(f, divisor)
		}
		base := new(big.Float).SetInt(binaryBase)
		f.Mul(f, base)
		// Convert to integer (truncate)
		result, _ := f.Int(nil)
		return Quantity{raw: raw, unscaled: result, scale: 0}, nil
	}

	// Check for decimal SI suffix (E, P, T, G, M, k, m, u, n)
	if multiplierScale, suffix, ok := parseDecimalSuffix(s); ok {
		numStr := s[:len(s)-len(suffix)]
		unscaled, scale, err := parseDecimalString(numStr)
		if err != nil {
			return Quantity{}, fmt.Errorf("failed to parse quantity %q: %w", s, err)
		}
		// Adjust scale: if suffix means *10^X, we subtract X from scale
		newScale := scale - multiplierScale
		return Quantity{raw: raw, unscaled: unscaled, scale: newScale}, nil
	}

	// Check for exponent notation (e.g., "1e3")
	if idx := strings.IndexAny(s, "eE"); idx > 0 {
		mantissa := s[:idx]
		expStr := s[idx+1:]
		unscaled, scale, err := parseDecimalString(mantissa)
		if err != nil {
			return Quantity{}, fmt.Errorf("failed to parse quantity %q: %w", s, err)
		}

		exp := int32(0)
		negative := false
		if len(expStr) > 0 && expStr[0] == '-' {
			negative = true
			expStr = expStr[1:]
		} else if len(expStr) > 0 && expStr[0] == '+' {
			expStr = expStr[1:]
		}
		for _, c := range expStr {
			if c < '0' || c > '9' {
				return Quantity{}, fmt.Errorf("invalid exponent in quantity %q", s)
			}
			exp = exp*10 + int32(c-'0')
		}
		if negative {
			exp = -exp
		}

		// Adjust scale
		newScale := scale - exp
		return Quantity{raw: raw, unscaled: unscaled, scale: newScale}, nil
	}

	// Plain number (integer or decimal)
	unscaled, scale, err := parseDecimalString(s)
	if err != nil {
		return Quantity{}, fmt.Errorf("failed to parse quantity %q: %w", s, err)
	}
	return Quantity{raw: raw, unscaled: unscaled, scale: scale}, nil
}

// parseDecimalString parses a decimal number string like "123" or "1.5" into
// (unscaled value, scale). "1.5" => (15, 1), "100" => (100, 0).
func parseDecimalString(s string) (*big.Int, int32, error) {
	if s == "" {
		return nil, 0, fmt.Errorf("empty number string")
	}

	negative := false
	if s[0] == '-' {
		negative = true
		s = s[1:]
	} else if s[0] == '+' {
		s = s[1:]
	}

	dotIdx := strings.IndexByte(s, '.')
	var intPart, fracPart string
	var scale int32

	if dotIdx >= 0 {
		intPart = s[:dotIdx]
		fracPart = s[dotIdx+1:]
		scale = int32(len(fracPart))
	} else {
		intPart = s
	}

	combined := intPart + fracPart
	if combined == "" {
		return nil, 0, fmt.Errorf("empty number")
	}

	value := new(big.Int)
	if _, ok := value.SetString(combined, 10); !ok {
		return nil, 0, fmt.Errorf("invalid number %q", combined)
	}

	if negative {
		value.Neg(value)
	}

	return value, scale, nil
}

// parseBinarySuffix checks if the string ends with a binary SI suffix.
// Returns (base multiplier, suffix string, ok).
func parseBinarySuffix(s string) (*big.Int, string, bool) {
	binarySuffixes := map[string]int64{
		"Ki": 1 << 10,
		"Mi": 1 << 20,
		"Gi": 1 << 30,
		"Ti": 1 << 40,
		"Pi": 1 << 50,
		"Ei": 1 << 60,
	}

	for suffix, base := range binarySuffixes {
		if strings.HasSuffix(s, suffix) && len(s) > len(suffix) {
			return big.NewInt(base), suffix, true
		}
	}
	return nil, "", false
}

// parseDecimalSuffix checks if the string ends with a decimal SI suffix.
// Returns (exponent, suffix string, ok).
func parseDecimalSuffix(s string) (int32, string, bool) {
	// Order matters: check longer suffixes first to avoid false matches
	decimalSuffixes := map[string]int32{
		"E": 18,
		"P": 15,
		"T": 12,
		"G": 9,
		"M": 6,
		"k": 3,
		"m": -3,
		"u": -6,
		"n": -9,
	}

	if len(s) < 2 {
		return 0, "", false
	}

	// Last character could be a suffix
	last := s[len(s)-1:]

	// Make sure the character before suffix is a digit or dot (to avoid matching
	// things like just "m" or "E" that are standalone)
	prev := s[len(s)-2]
	if !isDigitOrDot(prev) {
		return 0, "", false
	}

	if exp, ok := decimalSuffixes[last]; ok {
		return exp, last, true
	}

	return 0, "", false
}

func isDigitOrDot(c byte) bool {
	return (c >= '0' && c <= '9') || c == '.'
}

// ParseQuantityString parses a Kubernetes quantity string and returns a Quantity.
func ParseQuantityString(s string) (Quantity, error) {
	return parseQuantity(s)
}

// MustParseQuantity parses a Kubernetes quantity string, panicking on failure.
// Intended for use in tests and constants only.
func MustParseQuantity(s string) Quantity {
	q, err := parseQuantity(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse quantity %q: %v", s, err))
	}
	return q
}
