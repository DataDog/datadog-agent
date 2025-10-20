# Java Mergeability Implementation Guide

## 🎯 Overview

This guide shows how to implement the Java approach to pattern merging in your Go library. The Java approach uses **token-level mergeability** with discrete levels instead of continuous similarity scoring.

## 🧠 Key Discovery: How Java Actually Works

After thorough analysis of the Java codebase, the Java approach uses a **two-phase process**:

### Phase 1: Real-time Document Processing
- **Tokenization**: Each log is tokenized using `DefaultLuceneTokenizingAutomatonBuilder`
- **Clustering Key**: Uses `PatternClusteringKey` which only considers:
  - **Tags** (metadata) 
  - **Token count** (number of tokens)
- **Real-time Merging**: Documents with same clustering key go to same bucket, but **only merge if tokens are identical**

### Phase 2: Batch Consolidation (The Magic!)
The key insight is in `MergeableRootNode.mergeClusters()` - this is where wildcards are actually created:

```java
// Groups clusters by (tags, token_count)
clusters.stream()
  .collect(Collectors.groupingBy(
      cluster -> Pair.of(cluster.getTags(), cluster.getRootToken().size())))
  .values()
  .forEach(similarClusters -> {
    // For each group of similar clusters...
    while (!similarClusters.isEmpty()) {
      final MergeableNode<T, A> cluster = similarClusters.remove(similarClusters.size() - 1);
      final ListIterator<MergeableNode<T, A>> iter = similarClusters.listIterator();
      while (iter.hasNext()) {
        final MergeableNode<T, A> candidate = iter.next();
        if (cluster.mergeTokensIfFits(candidate)) {
          iter.remove(); // Merge successful!
        }
      }
    }
  });
```

### The `possiblyWildcard` Flag
- **Only Word tokens with numeric patterns have `possiblyWildcard = true`**
- This means **only words like `user123`, `session456` can merge into wildcards**
- Generic words like `bob`, `cat` are **not mergeable** and stay separate
- The `WildcardableWord.mergeWith()` method handles the actual wildcard creation

### Example: `user123 logged in successfully` vs `user456 logged in successfully`
1. **Tokenization**: Both become `[Word("user123"), Word("logged"), Word("in"), Word("successfully")]`
2. **Clustering Key**: Both get `(tags, 4)` → Same bucket
3. **Real-time**: Can't merge (different tokens)
4. **Batch Consolidation**: 
   - `Word("user123")` vs `Word("user456")` → `MERGEABLE_AS_WILDCARD` (both have numeric patterns)
   - `Word("logged")` vs `Word("logged")` → `FITS_AS_IT_IS` (same text)
   - `Word("in")` vs `Word("in")` → `FITS_AS_IT_IS` (same text)
   - `Word("successfully")` vs `Word("successfully")` → `FITS_AS_IT_IS` (same text)
5. **Result**: Pattern becomes `[user* logged in successfully]`

### Example: `bob loves eat 25` vs `cat loves eat 62`
1. **Tokenization**: Both become `[Word("bob"), Word("loves"), Word("eat"), NumericValue(25)]`
2. **Clustering Key**: Both get `(tags, 4)` → Same bucket
3. **Real-time**: Can't merge (different tokens)
4. **Batch Consolidation**: 
   - `Word("bob")` vs `Word("cat")` → `UNMERGEABLE` (generic words, no numeric patterns)
   - **Result**: Separate patterns (no merge) ✅

## 🏗️ Project Structure

```
your-go-library/
├── internal/
│   ├── token/
│   │   ├── token.go           # Token interface and MergeabilityLevel
│   │   ├── word.go            # Word token implementation
│   │   ├── numeric.go         # NumericValue token implementation
│   │   ├── special.go         # SpecialCharacter token implementation
│   │   └── token_list.go      # TokenList implementation
│   ├── tokenization/
│   │   ├── tokenizer.go       # Tokenization engine
│   │   └── parser.go          # Parser interface
│   ├── clustering/
│   │   ├── clusterer.go       # Clustering interfaces
│   │   ├── realtime.go        # RealTimeClusterer
│   │   └── consolidation.go   # Batch consolidation
│   └── patterns/
│       ├── extractor.go       # PatternExtractor
│       └── matcher.go         # Pattern matching
├── pkg/
│   └── patterns/
│       └── patterns.go        # Public API
└── go.mod
```

## 🚀 Implementation Steps

### Step 1: Core Token System (45 minutes)

**File: `internal/token/token.go`**

```go
package token

type Token interface {
    IsWildcard() bool
    GetPatternString() string
    GetMergeabilityLevel(other Token) MergeabilityLevel
    MergeWith(other Token) Token
}

type MergeabilityLevel int

const (
    UNMERGEABLE MergeabilityLevel = iota
    MERGEABLE_AS_NEW_TYPE
    MERGEABLE_AS_WILDCARD
    MERGEABLE_WITH_WIDER_RANGE
    FITS
    FITS_AS_IT_IS
)

func (m MergeabilityLevel) Compare(other MergeabilityLevel) int {
    return int(m) - int(other)
}

func (m MergeabilityLevel) IsMergeable() bool {
    return m > UNMERGEABLE
}
```

### Step 2: Word Token Implementation (30 minutes)

**File: `internal/token/word.go`**

```go
package token

type Word struct {
    text             string
    hasDigits        bool
    possiblyWildcard bool
    wildcardSummary  WildcardSummary
}

func NewWord(text string, possiblyWildcard, withSummaries bool) *Word {
    return &Word{
        text:             text,
        hasDigits:        containsDigits(text),
        possiblyWildcard: possiblyWildcard,
        wildcardSummary:  createWildcardSummary(text, withSummaries),
    }
}

func (w *Word) GetMergeabilityLevel(other Token) MergeabilityLevel {
    if otherWord, ok := other.(*Word); ok {
        return w.getMergeabilityWithWord(otherWord)
    } else if numericValue, ok := other.(*NumericValue); ok {
        return w.getMergeabilityWithNumeric(numericValue)
    }
    return UNMERGEABLE
}

func (w *Word) getMergeabilityWithWord(other *Word) MergeabilityLevel {
    if w.text != "" && other.text != "" {
        if w.text == other.text {
            if w.possiblyWildcard && !other.possiblyWildcard {
                return FITS
            }
            return FITS_AS_IT_IS
        } else if w.possiblyWildcard && other.possiblyWildcard {
            return MERGEABLE_AS_WILDCARD  // Both have numeric patterns
        } else {
            return UNMERGEABLE  // Generic words don't merge
        }
    }
    
    if w.possiblyWildcard {
        return MERGEABLE_AS_WILDCARD
    }
    
    return UNMERGEABLE  // Generic words are not mergeable
}

func (w *Word) MergeWith(other Token) Token {
    if otherWord, ok := other.(*Word); ok {
        return w.mergeWithWord(otherWord)
    } else if numericValue, ok := other.(*NumericValue); ok {
        return w.mergeWithNumeric(numericValue)
    }
    return w
}

func (w *Word) mergeWithWord(other *Word) *Word {
    merged := &Word{
        text:             w.text,
        hasDigits:        w.hasDigits || other.hasDigits,
        possiblyWildcard: w.possiblyWildcard,
        wildcardSummary:  w.wildcardSummary,
    }
    
    // If both have text and they're different, make wildcard
    if w.text != "" && other.text != "" && w.text != other.text {
        merged.possiblyWildcard = true
        merged.wildcardSummary = mergeWildcardSummaries(w.wildcardSummary, other.wildcardSummary)
    }
    
    return merged
}
```

### Step 3: TokenList Implementation (20 minutes)

**File: `internal/token/token_list.go`**

```go
package token

type TokenList struct {
    tokens []Token
}

func NewTokenList(tokens []Token) *TokenList {
    return &TokenList{tokens: tokens}
}

func (tl *TokenList) GetMergeabilityLevel(other Token) MergeabilityLevel {
    otherList, ok := other.(*TokenList)
    if !ok {
        return UNMERGEABLE
    }
    
    if len(tl.tokens) != len(otherList.tokens) {
        return UNMERGEABLE
    }
    
    minLevel := FITS_AS_IT_IS
    for i := 0; i < len(tl.tokens); i++ {
        level := tl.tokens[i].GetMergeabilityLevel(otherList.tokens[i])
        if level.Compare(minLevel) < 0 {
            if level == UNMERGEABLE {
                return UNMERGEABLE
            }
            minLevel = level
        }
    }
    return minLevel
}

func (tl *TokenList) MergeWith(other Token) Token {
    otherList := other.(*TokenList)
    mergedTokens := make([]Token, len(tl.tokens))
    for i := 0; i < len(tl.tokens); i++ {
        mergedTokens[i] = tl.tokens[i].MergeWith(otherList.tokens[i])
    }
    return NewTokenList(mergedTokens)
}
```

### Step 4: Two-Phase Clustering System (60 minutes)

**File: `internal/clustering/realtime.go`**

```go
package clustering

import (
    "sync"
    "github.com/your-library/internal/token"
)

type ClusteringKey struct {
    Tags       map[string]interface{}
    TokenCount int
}

type RealTimeClusterer struct {
    clusters map[ClusteringKey][]*MergeableNode
    mutex    sync.RWMutex
}

type MergeableNode struct {
    rootToken    *token.TokenList
    messages     []string
    count        int
    tags         map[string]interface{}
}

func NewRealTimeClusterer() *RealTimeClusterer {
    return &RealTimeClusterer{
        clusters: make(map[ClusteringKey][]*MergeableNode),
    }
}

func (rtc *RealTimeClusterer) ProcessDocument(message string, rootToken *token.TokenList, tags map[string]interface{}) *MergeableNode {
    key := ClusteringKey{
        Tags:       tags,
        TokenCount: len(rootToken.GetTokens()),
    }
    
    rtc.mutex.Lock()
    defer rtc.mutex.Unlock()
    
    // Try to find existing cluster that can accept this document
    if clusters, exists := rtc.clusters[key]; exists {
        for _, cluster := range clusters {
            if cluster.ProcessIfMergeable(rootToken) {
                cluster.AddMessage(message)
                return cluster
            }
        }
    }
    
    // Create new cluster
    newCluster := &MergeableNode{
        rootToken: rootToken,
        messages:  []string{message},
        count:     1,
        tags:      tags,
    }
    rtc.clusters[key] = append(rtc.clusters[key], newCluster)
    return newCluster
}

func (mn *MergeableNode) ProcessIfMergeable(rootToken *token.TokenList) bool {
    if mn.rootToken.GetMergeabilityLevel(rootToken).IsMergeable() {
        mn.rootToken = mn.rootToken.MergeWith(rootToken).(*token.TokenList)
        return true
    }
    return false
}

func (mn *MergeableNode) AddMessage(message string) {
    mn.messages = append(mn.messages, message)
    mn.count++
}
```

### Step 5: Batch Consolidation (45 minutes)

**File: `internal/clustering/consolidation.go`**

```go
package clustering

func (rtc *RealTimeClusterer) ConsolidateClusters() []*MergeableNode {
    rtc.mutex.Lock()
    defer rtc.mutex.Unlock()
    
    var consolidatedClusters []*MergeableNode
    
    // Group clusters by (tags, token_count) - same as Java
    for _, clusters := range rtc.clusters {
        consolidatedClusters = append(consolidatedClusters, 
            rtc.mergeClusters(clusters)...)
    }
    
    return consolidatedClusters
}

func (rtc *RealTimeClusterer) mergeClusters(clusters []*MergeableNode) []*MergeableNode {
    var consolidatedClusters []*MergeableNode
    
    // Java-style consolidation algorithm
    for len(clusters) > 0 {
        cluster := clusters[len(clusters)-1]
        clusters = clusters[:len(clusters)-1]
        
        var remainingClusters []*MergeableNode
        for _, candidate := range clusters {
            if cluster.MergeTokensIfFits(candidate) {
                // Merge successful, candidate is absorbed
                continue
            } else if candidate.MergeTokensIfFits(cluster) {
                // Candidate can absorb cluster, use candidate as base
                cluster = candidate
                continue
            } else {
                // No merge possible, keep candidate
                remainingClusters = append(remainingClusters, candidate)
            }
        }
        
        consolidatedClusters = append(consolidatedClusters, cluster)
        clusters = remainingClusters
    }
    
    return consolidatedClusters
}

func (mn *MergeableNode) MergeTokensIfFits(other *MergeableNode) bool {
    if mn.rootToken.GetMergeabilityLevel(other.rootToken).IsMergeable() {
        mn.rootToken = mn.rootToken.MergeWith(other.rootToken).(*token.TokenList)
        mn.messages = append(mn.messages, other.messages...)
        mn.count += other.count
        return true
    }
    return false
}
```

### Step 6: Pattern Extractor Integration (30 minutes)

**File: `pkg/patterns/patterns.go`**

```go
package patterns

import (
    "github.com/your-library/internal/clustering"
    "github.com/your-library/internal/tokenization"
)

type PatternExtractor struct {
    tokenizer tokenization.Tokenizer
    clusterer *clustering.RealTimeClusterer
}

func NewPatternExtractor() *PatternExtractor {
    return &PatternExtractor{
        tokenizer: tokenization.NewDefaultTokenizer(),
        clusterer: clustering.NewRealTimeClusterer(),
    }
}

func (pe *PatternExtractor) ExtractPatterns(messages []string) ([]*Pattern, error) {
    // Phase 1: Real-time processing
    for _, message := range messages {
        tokens, err := pe.tokenizer.Tokenize(message)
        if err != nil {
            return nil, err
        }
        
        tokenList := token.NewTokenList(tokens)
        pe.clusterer.ProcessDocument(message, tokenList, make(map[string]interface{}))
    }
    
    // Phase 2: Batch consolidation
    clusters := pe.clusterer.ConsolidateClusters()
    
    // Convert to patterns
    patterns := make([]*Pattern, len(clusters))
    for i, cluster := range clusters {
        patterns[i] = &Pattern{
            Template: cluster.rootToken.GetPatternString(),
            Count:    cluster.count,
            Messages: cluster.messages,
        }
    }
    
    return patterns, nil
}

type Pattern struct {
    Template string
    Count    int
    Messages []string
}
```

### Step 7: Basic Tokenization (30 minutes)

**File: `internal/tokenization/tokenizer.go`**

```go
package tokenization

import (
    "strconv"
    "strings"
    "github.com/your-library/internal/token"
)

type Tokenizer interface {
    Tokenize(input string) ([]token.Token, error)
}

type DefaultTokenizer struct{}

func NewDefaultTokenizer() *DefaultTokenizer {
    return &DefaultTokenizer{}
}

func (dt *DefaultTokenizer) Tokenize(input string) ([]token.Token, error) {
    var tokens []token.Token
    
    // Simple word-based tokenization
    words := strings.Fields(input)
    for _, word := range words {
        if isNumeric(word) {
            tokens = append(tokens, token.NewNumericValue(word, false))
        } else if hasNumericPattern(word) {
            // Only words with numeric patterns can be wildcards
            tokens = append(tokens, token.NewWord(word, true, false)) // possiblyWildcard=true
        } else {
            // Generic words are not mergeable
            tokens = append(tokens, token.NewWord(word, false, false)) // possiblyWildcard=false
        }
    }
    
    return tokens, nil
}

func hasNumericPattern(word string) bool {
    // Check if word contains numbers (user123, session456, etc.)
    return regexp.MustCompile(`\d`).MatchString(word)
}

func isNumeric(s string) bool {
    _, err := strconv.ParseFloat(s, 64)
    return err == nil
}
```

## 🧪 Testing Implementation (30 minutes)

**File: `internal/token/word_test.go`**

```go
package token

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestWordMergeability(t *testing.T) {
    tests := []struct {
        name     string
        token1   *Word
        token2   *Word
        expected MergeabilityLevel
    }{
        {
            name:     "Same text, both wildcard",
            token1:   NewWord("GET", true, false),
            token2:   NewWord("GET", true, false),
            expected: FITS_AS_IT_IS,
        },
        {
            name:     "Different text, both wildcard",
            token1:   NewWord("GET", true, false),
            token2:   NewWord("POST", true, false),
            expected: MERGEABLE_AS_WILDCARD,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := tt.token1.GetMergeabilityLevel(tt.token2)
            assert.Equal(t, tt.expected, result)
        })
    }
}

func TestPatternClustering(t *testing.T) {
    extractor := NewPatternExtractor()
    
    logMessages := []string{
        "user123 logged in successfully",
        "user456 logged in successfully", 
        "user789 logged in successfully",
    }
    
    patterns, err := extractor.ExtractPatterns(logMessages)
    require.NoError(t, err)
    
    // Should create one pattern with wildcards
    assert.Len(t, patterns, 1)
    assert.Equal(t, "user* logged in successfully", patterns[0].Template)
    assert.Equal(t, 3, patterns[0].Count)
}
```

## 📦 Go Module Setup (5 minutes)

**File: `go.mod`**

```go
module github.com/your-org/your-patterns-library

go 1.21

require (
    github.com/stretchr/testify v1.8.4
)
```

## 🎯 Key Differences from Go Approach

### Go Approach (Current)
- **Similarity-based**: Uses Jaccard similarity with 50% threshold
- **Single-phase**: All processing happens in real-time
- **Continuous scoring**: Similarity values between 0.0 and 1.0
- **Constant word similarity**: Additional check prevents merging very different patterns

### Java Approach (Proposed)
- **Mergeability-based**: Uses discrete mergeability levels
- **Two-phase**: Real-time processing + batch consolidation
- **Binary decisions**: Either mergeable or not mergeable
- **Token-level rules**: Each token type defines its own mergeability logic
- **`possiblyWildcard` flag**: Enables wildcard creation for different word tokens

### Why Java Approach Works Better

1. **No Similarity Thresholds**: The `possiblyWildcard` flag eliminates the need for similarity calculations
2. **Batch Optimization**: Consolidation happens after all documents are processed, allowing better pattern discovery
3. **Predictable Behavior**: Discrete levels make the system more debuggable
4. **Performance**: O(1) token-level checks vs O(n²) similarity calculations
5. **Semantic Awareness**: Different token types have different mergeability rules

## ✅ Benefits of This Implementation

1. **Performance**: O(1) token-level checks vs O(n²) similarity calculations
2. **Predictability**: Discrete mergeability levels make behavior more predictable
3. **Type Safety**: Each token type defines its own mergeability rules
4. **Extensibility**: Easy to add new token types with custom mergeability
5. **Semantic Awareness**: Can distinguish between different types of content
6. **Backward Compatibility**: Can fall back to Go approach if needed
7. **Wildcard Creation**: The `possiblyWildcard` flag enables automatic wildcard creation during batch consolidation

## 🎯 Summary

**Total Estimated Time: ~4 hours for complete implementation from scratch**

This implementation provides a high-performance, predictable pattern merging system that scales well under load while maintaining semantic awareness of different token types. The discrete mergeability levels make the system more maintainable and debuggable compared to continuous similarity scoring.

**The key insight is the `possiblyWildcard` flag that enables automatic wildcard creation during batch consolidation, eliminating the need for similarity thresholds.**