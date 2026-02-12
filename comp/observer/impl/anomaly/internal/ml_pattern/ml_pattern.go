// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ml_pattern provides machine learning based pattern matching for log messages.
package ml_pattern

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/DataDog/datadog-agent/comp/observer/impl/anomaly/internal/types"
)

// MLPatternDetector implements machine learning-based pattern detection
type MLPatternDetector struct {
	minClusterSize      int     // Minimum size for a cluster to be considered valid
	similarityThreshold float64 // Threshold for cosine similarity (0-1)
}

// NewMLPatternDetector creates a new ML-based pattern detector
func NewMLPatternDetector() *MLPatternDetector {
	return &MLPatternDetector{
		minClusterSize:      2,
		similarityThreshold: 0.7, // 70% similarity threshold
	}
}

// Document represents a preprocessed log message
type Document struct {
	ID       int
	Original types.LogError
	Tokens   []string
	Vector   map[string]float64 // TF-IDF vector
}

// Cluster represents a group of similar documents
type Cluster struct {
	ID        int
	Centroid  map[string]float64
	Documents []Document
	Count     int64
	Pattern   string
}

// normalizeDynamicValues replaces dynamic values with placeholders for better clustering
func (ml *MLPatternDetector) normalizeDynamicValues(message string) string {
	// URLs (must be before paths and IPs to avoid breaking up the URL)
	// Match URLs but stop at brackets, quotes, or spaces
	urlPattern := regexp.MustCompile(`https?://[^\s\[\]"']+`)
	message = urlPattern.ReplaceAllString(message, "<URL>")

	// UUID patterns (e.g., ac8218cf-498b-4d33-bd44-151095959547)
	uuidPattern := regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	message = uuidPattern.ReplaceAllString(message, "<UUID>")

	// Timestamps with various formats
	// Full datetime with milliseconds and timezone: "2026-01-16 13:04:30:064 +0000"
	fullDateTimePattern := regexp.MustCompile(`\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}[:.]\d{3,6}\s*[+-]\d{4}`)
	message = fullDateTimePattern.ReplaceAllString(message, "<TIMESTAMP>")

	// Human-readable format: "Jan 16, 2026 1:03:01 PM" or "January 16, 2026 1:03:01 PM"
	humanDatePattern := regexp.MustCompile(`\b(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\s+\d{1,2},?\s+\d{4}\s+\d{1,2}:\d{2}:\d{2}\s*(?:AM|PM)?\b`)
	message = humanDatePattern.ReplaceAllString(message, "<TIMESTAMP>")

	// ISO8601 with milliseconds: 2026-01-16T13:04:30.064 or 2026-01-16 13:04:30.064
	timestampWithMs := regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}[:.]\d{3,6}`)
	message = timestampWithMs.ReplaceAllString(message, "<TIMESTAMP>")

	// ISO8601: 2026-01-16T13:04:30 or 2026-01-16 13:04:30
	timestampPattern1 := regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}`)
	message = timestampPattern1.ReplaceAllString(message, "<TIMESTAMP>")

	// Time with milliseconds/microseconds: 13:04:30:064 or 13:04:30.064
	timestampPattern2 := regexp.MustCompile(`\d{2}:\d{2}:\d{2}[:.]\d{3,6}`)
	message = timestampPattern2.ReplaceAllString(message, "<TIMESTAMP>")

	// Timezone offset: +0000, -0500, etc.
	timezonePattern := regexp.MustCompile(`[+-]\d{4}\b`)
	message = timezonePattern.ReplaceAllString(message, "<TZ>")

	// Time with AM/PM: 1:03:01 PM or 13:04:30
	timeWithAMPM := regexp.MustCompile(`\b\d{1,2}:\d{2}:\d{2}\s*(?:AM|PM)?\b`)
	message = timeWithAMPM.ReplaceAllString(message, "<TIME>")

	// Hash patterns (e.g., hashes.usr.id:>=-3074457345618258604)
	// Negative or positive large numbers (likely hashes or IDs)
	hashPattern := regexp.MustCompile(`-?\d{10,}`)
	message = hashPattern.ReplaceAllString(message, "<HASH>")

	// IPv4 addresses
	ipv4Pattern := regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
	message = ipv4Pattern.ReplaceAllString(message, "<IP>")

	// Hex strings (e.g., 0x1a2b3c4d or just long hex sequences)
	hexPattern := regexp.MustCompile(`\b(?:0x)?[0-9a-f]{8,}\b`)
	message = hexPattern.ReplaceAllString(message, "<HEX>")

	// Session IDs and similar alphanumeric identifiers (mixed case, 20+ chars)
	sessionPattern := regexp.MustCompile(`\b[A-Za-z0-9]{20,}\b`)
	message = sessionPattern.ReplaceAllString(message, "<SESSION>")

	// Duration values (e.g., "5 minutes", "10 seconds")
	durationPattern := regexp.MustCompile(`\b\d+\s*(minutes?|seconds?|hours?|days?|ms|milliseconds?)\b`)
	message = durationPattern.ReplaceAllString(message, "<DURATION>")

	// HTTP status codes (e.g., "403", "Status: 403")
	statusPattern := regexp.MustCompile(`\b(status:\s*)?\d{3}\b`)
	message = statusPattern.ReplaceAllString(message, "<STATUS>")

	// File paths (unix-style and windows-style)
	pathPattern := regexp.MustCompile(`(?:/[\w\-./]+)+|(?:[A-Z]:\\[\w\-\\]+)`)
	message = pathPattern.ReplaceAllString(message, "<PATH>")

	// Email addresses
	emailPattern := regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`)
	message = emailPattern.ReplaceAllString(message, "<EMAIL>")

	return message
}

// Tokenize splits a message into tokens (words) and normalizes dynamic values
func (ml *MLPatternDetector) Tokenize(message string) []string {
	// Convert to lowercase
	message = strings.ToLower(message)

	// Normalize dynamic values before tokenization
	message = ml.normalizeDynamicValues(message)

	// Replace common separators with spaces (including underscore for word boundary matching)
	replacer := strings.NewReplacer(
		"|", " ",
		":", " ",
		",", " ",
		";", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		"{", " ",
		"}", " ",
		"/", " ",
		"\\", " ",
		"\"", " ",
		"'", " ",
		"_", " ", // Treat underscore as word boundary
		"-", " ", // Treat hyphen as word boundary
		".", " ", // Treat period as word boundary
	)
	message = replacer.Replace(message)

	// Split by whitespace
	tokens := strings.Fields(message)

	// Filter tokens
	var filtered []string
	for _, token := range tokens {
		if len(token) < 2 || isPureNumber(token) || isStopWord(token) {
			continue
		}
		filtered = append(filtered, token)
	}

	return filtered
}

// isPureNumber checks if a string contains only digits and dots
func isPureNumber(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) && r != '.' {
			return false
		}
	}
	return true
}

// isStopWord checks if a token is a common stop word
func isStopWord(token string) bool {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
		"is": true, "was": true, "are": true, "were": true, "be": true,
		"been": true, "being": true, "have": true, "has": true, "had": true,
		"do": true, "does": true, "did": true, "will": true, "would": true,
		"could": true, "should": true, "may": true, "might": true, "must": true,
		"can": true, "this": true, "that": true, "these": true, "those": true,
	}
	return stopWords[token]
}

// ComputeTFIDF computes TF-IDF vectors for all documents
func (ml *MLPatternDetector) ComputeTFIDF(documents []Document) []Document {
	// Calculate document frequency (DF) for each term
	df := make(map[string]int)
	for _, doc := range documents {
		seen := make(map[string]bool)
		for _, token := range doc.Tokens {
			if !seen[token] {
				df[token]++
				seen[token] = true
			}
		}
	}

	numDocs := float64(len(documents))

	// Calculate TF-IDF for each document
	for i := range documents {
		doc := &documents[i]

		// Calculate term frequency (TF)
		tf := make(map[string]float64)
		for _, token := range doc.Tokens {
			tf[token]++
		}

		// Normalize TF by document length
		docLength := float64(len(doc.Tokens))
		if docLength > 0 {
			for token := range tf {
				tf[token] = tf[token] / docLength
			}
		}

		// Calculate TF-IDF
		tfidf := make(map[string]float64)
		for token, tfValue := range tf {
			idf := math.Log(numDocs / float64(df[token]))
			tfidf[token] = tfValue * idf
		}

		doc.Vector = tfidf
	}

	return documents
}

// CosineSimilarity computes cosine similarity between two TF-IDF vectors
func (ml *MLPatternDetector) CosineSimilarity(vec1, vec2 map[string]float64) float64 {
	// Calculate dot product
	dotProduct := 0.0
	for term, val1 := range vec1 {
		if val2, exists := vec2[term]; exists {
			dotProduct += val1 * val2
		}
	}

	// Calculate magnitudes
	mag1 := 0.0
	for _, val := range vec1 {
		mag1 += val * val
	}
	mag1 = math.Sqrt(mag1)

	mag2 := 0.0
	for _, val := range vec2 {
		mag2 += val * val
	}
	mag2 = math.Sqrt(mag2)

	// Avoid division by zero
	if mag1 == 0 || mag2 == 0 {
		return 0
	}

	return dotProduct / (mag1 * mag2)
}

// ComputeCentroid computes the centroid of a cluster
func (ml *MLPatternDetector) ComputeCentroid(documents []Document) map[string]float64 {
	centroid := make(map[string]float64)

	// Sum all vectors
	for _, doc := range documents {
		for term, value := range doc.Vector {
			centroid[term] += value
		}
	}

	// Average by number of documents
	numDocs := float64(len(documents))
	for term := range centroid {
		centroid[term] /= numDocs
	}

	return centroid
}

// GeneratePattern generates a representative pattern for a cluster
func (ml *MLPatternDetector) GeneratePattern(cluster *Cluster) string {
	// Find the most important terms in the centroid
	type termScore struct {
		term  string
		score float64
	}

	var scores []termScore
	for term, score := range cluster.Centroid {
		scores = append(scores, termScore{term, score})
	}

	// Sort by score descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	// Take top N terms
	topN := 10
	if len(scores) < topN {
		topN = len(scores)
	}

	var topTerms []string
	for i := 0; i < topN; i++ {
		topTerms = append(topTerms, scores[i].term)
	}

	return strings.Join(topTerms, " ")
}

// DetectPatternsDBSCAN performs density-based clustering using DBSCAN algorithm
func (ml *MLPatternDetector) DetectPatternsDBSCAN(logErrors []types.LogError, eps float64, minPts int) []types.LogPattern {
	// Preprocess documents
	documents := make([]Document, len(logErrors))
	for i, logError := range logErrors {
		tokens := ml.Tokenize(logError.Message)
		documents[i] = Document{
			ID:       i,
			Original: logError,
			Tokens:   tokens,
		}
	}

	// Compute TF-IDF vectors
	documents = ml.ComputeTFIDF(documents)

	// DBSCAN clustering
	clusters := ml.DBSCAN(documents, eps, minPts)

	// Convert clusters to LogPattern format
	var patterns []types.LogPattern
	for _, cluster := range clusters {
		// Skip noise cluster (ID = -1)
		if cluster.ID == -1 {
			continue
		}

		// Collect example messages (up to 3)
		var examples []string
		maxExamples := 3
		for i := 0; i < len(cluster.Documents) && i < maxExamples; i++ {
			examples = append(examples, cluster.Documents[i].Original.Message)
		}

		patterns = append(patterns, types.LogPattern{
			Pattern:     cluster.Pattern,
			Count:       cluster.Count,
			Examples:    examples,
			GroupingKey: cluster.Pattern,
		})
	}

	// Sort by count (descending)
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Count > patterns[j].Count
	})

	return patterns
}

// DBSCAN implements the DBSCAN clustering algorithm
func (ml *MLPatternDetector) DBSCAN(documents []Document, eps float64, minPts int) []Cluster {
	const UNCLASSIFIED = -2
	const NOISE = -1

	// Initialize cluster assignments
	clusterID := make([]int, len(documents))
	for i := range clusterID {
		clusterID[i] = UNCLASSIFIED
	}

	currentCluster := 0

	// Process each point
	for i := range documents {
		if clusterID[i] != UNCLASSIFIED {
			continue
		}

		// Find neighbors
		neighbors := ml.FindNeighbors(documents, i, eps)

		if len(neighbors) < minPts {
			clusterID[i] = NOISE
		} else {
			ml.ExpandCluster(documents, clusterID, i, neighbors, currentCluster, eps, minPts)
			currentCluster++
		}
	}

	// Build clusters from assignments
	clusterMap := make(map[int]*Cluster)
	for i, cid := range clusterID {
		if cid == NOISE {
			cid = -1
		}

		if _, exists := clusterMap[cid]; !exists {
			clusterMap[cid] = &Cluster{
				ID:        cid,
				Documents: []Document{},
				Count:     0,
			}
		}

		clusterMap[cid].Documents = append(clusterMap[cid].Documents, documents[i])
		clusterMap[cid].Count += documents[i].Original.Count
	}

	// Convert map to slice and compute centroids and patterns
	var clusters []Cluster
	for _, cluster := range clusterMap {
		cluster.Centroid = ml.ComputeCentroid(cluster.Documents)
		cluster.Pattern = ml.GeneratePattern(cluster)
		clusters = append(clusters, *cluster)
	}

	return clusters
}

// FindNeighbors finds all documents within eps distance from document i
func (ml *MLPatternDetector) FindNeighbors(documents []Document, i int, eps float64) []int {
	var neighbors []int
	for j := range documents {
		if i == j {
			continue
		}

		similarity := ml.CosineSimilarity(documents[i].Vector, documents[j].Vector)
		distance := 1.0 - similarity

		if distance <= eps {
			neighbors = append(neighbors, j)
		}
	}
	return neighbors
}

// ExpandCluster expands a cluster from a seed point
func (ml *MLPatternDetector) ExpandCluster(documents []Document, clusterID []int, seedIdx int,
	neighbors []int, currentCluster int, eps float64, minPts int) {

	const UNCLASSIFIED = -2
	const NOISE = -1

	clusterID[seedIdx] = currentCluster

	// Process neighbors
	for i := 0; i < len(neighbors); i++ {
		neighborIdx := neighbors[i]

		if clusterID[neighborIdx] == NOISE {
			clusterID[neighborIdx] = currentCluster
		}

		if clusterID[neighborIdx] != UNCLASSIFIED {
			continue
		}

		clusterID[neighborIdx] = currentCluster

		// Find neighbors of neighbor
		newNeighbors := ml.FindNeighbors(documents, neighborIdx, eps)
		if len(newNeighbors) >= minPts {
			neighbors = append(neighbors, newNeighbors...)
		}
	}
}
