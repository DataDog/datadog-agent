// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package namer

import (
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"
)

const nameSep = "-"

// In case of truncation, size of the hash suffix in percentage of the full size
const hashSizePercent = 20

// Used only for tests to skip hash suffix to ease test results validation
var noHash = false

type Namer struct {
	ctx      *pulumi.Context
	prefixes []string
}

func NewNamer(ctx *pulumi.Context, prefix string) Namer {
	if prefix == "" {
		return Namer{
			ctx:      ctx,
			prefixes: []string{},
		}
	}
	return Namer{
		ctx:      ctx,
		prefixes: []string{prefix},
	}
}

func (n Namer) WithPrefix(prefix string) Namer {
	prefixes := make([]string, len(n.prefixes), len(n.prefixes)+1)
	copy(prefixes, n.prefixes)
	return Namer{
		ctx:      n.ctx,
		prefixes: append(prefixes, prefix),
	}
}

func (n Namer) ResourceName(parts ...string) string {
	if len(parts) == 0 {
		panic("Resource name requires at least one part to generate name")
	}

	return joinWithMaxLength(math.MaxInt, append(n.prefixes, parts...))
}

func (n Namer) DisplayName(maxLen int, parts ...pulumi.StringInput) pulumi.StringInput {
	var convertedParts []interface{}
	for _, part := range parts {
		convertedParts = append(convertedParts, part)
	}
	return pulumi.All(convertedParts...).ApplyT(func(args []interface{}) string {
		strArgs := make([]string, 1, 1+len(n.prefixes)+len(args))
		strArgs[0] = n.ctx.Stack()
		strArgs = append(strArgs, n.prefixes...)
		for _, arg := range args {
			strArgs = append(strArgs, arg.(string))
		}
		return joinWithMaxLength(maxLen, strArgs)
	}).(pulumi.StringOutput)
}

func joinWithMaxLength(maxLength int, tokens []string) string {
	sumTokensSize := lo.Sum(lo.Map(tokens, func(s string, _ int) int { return len(s) }))

	// Check if non-truncated concatenation fits inside maximum length
	if sumTokensSize+(len(tokens)-1)*len(nameSep) <= maxLength {
		return strings.Join(tokens, nameSep)
	}

	// If a truncation is needed, a hash will be needed
	fullhash := hash(tokens)

	// Compute the size of the hash suffix that will be appended to the output
	hash := fullhash
	hashSize := maxLength * hashSizePercent / 100
	if len(hash) > hashSize {
		hash = hash[:hashSize]
	} else {
		hashSize = len(hash)
	}

	// For test purpose, we have an option to completely strip the output of the hash suffix
	// At this point, `hashSize` is the size of the hash suffix without the dash separator
	// -len(nameSep) means that we want to also strip the dash
	if noHash {
		hashSize = -len(nameSep)
	}

	// If there’s so many tokens that truncating all of them to a single character string and keeping only the dash separators
	// would exceed the maximum length, we cannot do anything better than returning only the hash.
	if len(tokens)+(len(tokens))*len(nameSep)+hashSize > maxLength {
		if len(fullhash) > maxLength {
			return fullhash[:maxLength]
		}
		return fullhash
	}

	var sb strings.Builder

	// Truncate all tokens in the same relative proportion
	sumTruncatedTokensSize := maxLength - len(tokens)*len(nameSep) - hashSize
	tokenIdx := 0
	nextX := len(tokens[tokenIdx])
	prevY := 0
	bresenham(0, 0, sumTokensSize, sumTruncatedTokensSize, func(x, y int) {
		if x == nextX {
			sb.WriteString(tokens[tokenIdx][:y-prevY])
			sb.WriteString(nameSep)
			tokenIdx++
			if x < sumTokensSize {
				nextX += len(tokens[tokenIdx])
				prevY = y
			}
		}
	})

	if noHash {
		str := sb.String()
		return str[:len(str)-len(nameSep)] // Strip the trailing dash
	}

	sb.WriteString(hash)
	return sb.String()
}

func hash(tokens []string) string {
	hasher := fnv.New64a()
	for _, tok := range tokens {
		_, _ = io.WriteString(hasher, tok)
	}
	return fmt.Sprintf("%016x", hasher.Sum64())
}

// Bresenham’s algorithm restricted to the first octant
func bresenham(x, y, x2, y2 int, plot func(x, y int)) {
	d := x2 - x
	dx := 2 * d
	dy := 2 * (y2 - y)
	for x <= x2 {
		plot(x, y)
		x++
		d -= dy
		if d <= 0 {
			y++
			d += dx
		}
	}
}
