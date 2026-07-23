// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

// This file contains experiment-only analysis used to attribute accuracy
// differences between the Observer semantic tokenizer and the Logs tokenizer.
// It deliberately runs both token streams through the same fixed-representative
// positional matcher so tokenization is the only changing factor.

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	semanticpatterns "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl/patterns"
	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	logpattern "github.com/DataDog/datadog-agent/pkg/logs/pattern"
)

const attributionMinEmitCount = 5

var (
	attributionUUIDPattern = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\b`)
	attributionHexPattern  = regexp.MustCompile(`(?i)\b[0-9a-f]{16,}\b`)
)

type attributionGroup struct {
	Source  string `json:"source,omitempty"`
	Service string `json:"service,omitempty"`
	Env     string `json:"env,omitempty"`
	Host    string `json:"host,omitempty"`
}

func (g attributionGroup) key() string {
	return g.Source + "|" + g.Service + "|" + g.Env + "|" + g.Host
}

func (g attributionGroup) hash() string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(g.key()))
	return strconv.FormatUint(h.Sum64(), 16)
}

type attributionCluster struct {
	internalID int
	tokens     []string

	ID             string           `json:"id"`
	PatternHash    string           `json:"pattern_hash"`
	GroupHash      string           `json:"group_hash"`
	Group          attributionGroup `json:"group"`
	Pattern        string           `json:"pattern"`
	Example        string           `json:"example"`
	TokenTypes     []string         `json:"token_types,omitempty"`
	Constructs     []string         `json:"constructs,omitempty"`
	Count          int              `json:"count"`
	NonFeedback    int              `json:"non_feedback_count"`
	EmittedLogs    int              `json:"emitted_log_count"`
	FirstTimestamp int64            `json:"first_timestamp"`
	LastTimestamp  int64            `json:"last_timestamp"`
	PhaseCounts    map[string]int   `json:"phase_counts"`
}

type attributionClusterer struct {
	threshold float64
	nextID    int
	groups    map[string][]*attributionCluster
	clusters  []*attributionCluster
}

func newAttributionClusterer(threshold float64) *attributionClusterer {
	return &attributionClusterer{
		threshold: threshold,
		groups:    make(map[string][]*attributionCluster),
	}
}

func attributionTokensMatch(a, b []string, threshold float64) bool {
	count := min(len(a), len(b))
	if count == 0 {
		return len(a) == len(b)
	}
	required := int(math.Round(threshold * float64(count)))
	matches := 0
	for i := 0; i < count; i++ {
		if a[i] == b[i] {
			matches++
		}
		if matches+(count-i-1) < required {
			return false
		}
	}
	return true
}

func (c *attributionClusterer) process(
	group attributionGroup,
	tokens []string,
	patternHash string,
	pattern string,
	example string,
	tokenTypes []string,
	constructs []string,
	timestamp int64,
	phase string,
	feedback bool,
) (*attributionCluster, bool) {
	groupKey := group.key()
	patterns := c.groups[groupKey]
	matched := -1
	for i := range patterns {
		if attributionTokensMatch(patterns[i].tokens, tokens, c.threshold) {
			matched = i
			break
		}
	}
	if matched < 0 {
		c.nextID++
		cluster := &attributionCluster{
			internalID:     c.nextID,
			ID:             group.hash() + "/" + patternHash,
			PatternHash:    patternHash,
			GroupHash:      group.hash(),
			Group:          group,
			Pattern:        pattern,
			Example:        truncateAttribution(example, 300),
			TokenTypes:     tokenTypes,
			Constructs:     constructs,
			FirstTimestamp: timestamp,
			PhaseCounts:    make(map[string]int),
			tokens:         append([]string(nil), tokens...),
		}
		patterns = append(patterns, cluster)
		c.clusters = append(c.clusters, cluster)
		matched = len(patterns) - 1
	}

	cluster := patterns[matched]
	cluster.Count++
	cluster.LastTimestamp = timestamp
	cluster.PhaseCounts[phase]++
	if !feedback {
		cluster.NonFeedback++
	}
	if cluster.Count >= attributionMinEmitCount {
		cluster.EmittedLogs++
	}

	for matched > 0 && patterns[matched-1].Count < patterns[matched].Count {
		patterns[matched-1], patterns[matched] = patterns[matched], patterns[matched-1]
		matched--
	}
	c.groups[groupKey] = patterns
	return cluster, cluster.Count >= attributionMinEmitCount
}

type attributionMappingKey struct {
	semantic int
	logs     int
}

type attributionMapping struct {
	SemanticClusterID       string         `json:"semantic_cluster_id"`
	LogsClusterID           string         `json:"logs_cluster_id"`
	GroupHash               string         `json:"group_hash"`
	Count                   int            `json:"count"`
	PhaseCounts             map[string]int `json:"phase_counts"`
	Examples                []string       `json:"examples,omitempty"`
	SemanticEmittedLogs     int            `json:"semantic_emitted_logs"`
	LogsEmittedLogs         int            `json:"logs_emitted_logs"`
	SemanticOnlyEmittedLogs int            `json:"semantic_only_emitted_logs"`
	FeedbackLogs            int            `json:"feedback_logs"`
}

type attributionCandidate struct {
	Name                    string `json:"name"`
	Kind                    string `json:"kind"`
	Clusters                int    `json:"clusters"`
	Logs                    int    `json:"logs"`
	FragmentedLogs          int    `json:"fragmented_logs"`
	SemanticOnlyEmittedLogs int    `json:"semantic_only_emitted_logs"`
}

type attributionSummary struct {
	Logs                      int     `json:"logs"`
	FeedbackLogs              int     `json:"feedback_logs"`
	SemanticClusters          int     `json:"semantic_clusters"`
	LogsClusters              int     `json:"logs_clusters"`
	SemanticEmittedClusters   int     `json:"semantic_emitted_clusters"`
	LogsEmittedClusters       int     `json:"logs_emitted_clusters"`
	SemanticFragmentationRate float64 `json:"semantic_fragmentation_rate"`
	LogsOvermergeRate         float64 `json:"logs_overmerge_rate"`
	SemanticOnlyEmittedLogs   int     `json:"semantic_only_emitted_logs"`
	LogsOnlyEmittedLogs       int     `json:"logs_only_emitted_logs"`
	NonFeedbackComparedLogs   int     `json:"non_feedback_compared_logs"`
}

type patternAttributionReport struct {
	Scenario          string                 `json:"scenario"`
	MatchThreshold    float64                `json:"match_threshold"`
	MinEmitCount      int                    `json:"min_emit_count"`
	Comparison        string                 `json:"comparison"`
	Episode           *EpisodeInfo           `json:"episode,omitempty"`
	Summary           attributionSummary     `json:"summary"`
	SemanticClusters  []*attributionCluster  `json:"semantic_clusters"`
	LogsClusters      []*attributionCluster  `json:"logs_clusters"`
	Mappings          []*attributionMapping  `json:"mappings"`
	CandidateFeatures []attributionCandidate `json:"candidate_features"`
}

// RunPatternAttribution writes a controlled semantic-tokenizer versus
// Logs-tokenizer cluster mapping report for one scenario.
func RunPatternAttribution(scenariosDir, scenario, output string, threshold float64) error {
	if threshold <= 0 || threshold > 1 {
		return fmt.Errorf("match threshold must be in (0,1], got %v", threshold)
	}
	scenarioDir := filepath.Join(scenariosDir, scenario)
	parquetDir := filepath.Join(scenarioDir, "parquet")
	if _, err := os.Stat(parquetDir); err != nil {
		return fmt.Errorf("scenario parquet directory: %w", err)
	}

	var episode *EpisodeInfo
	if data, err := os.ReadFile(filepath.Join(scenarioDir, "episode.json")); err == nil {
		var parsed EpisodeInfo
		if json.Unmarshal(data, &parsed) == nil {
			episode = &parsed
		}
	}

	var logs []recorderdef.LogData
	var err error
	if detectParquetFormat(parquetDir) == FormatV2 {
		logs, err = readAllLogsV2(parquetDir)
	} else {
		logs, err = readAllLogs(parquetDir)
	}
	if err != nil {
		return fmt.Errorf("reading logs: %w", err)
	}

	semanticTokenizer := semanticpatterns.NewTokenizer()
	logsTokenizer := logpattern.NewTokenizer(12500)
	semanticClusterer := newAttributionClusterer(threshold)
	logsClusterer := newAttributionClusterer(threshold)
	mappings := make(map[attributionMappingKey]*attributionMapping)
	var summary attributionSummary
	summary.Logs = len(logs)

	for _, log := range logs {
		content := string(log.Content)
		if content == "" {
			continue
		}
		timestamp := log.TimestampMs / 1000
		phase := attributionPhase(timestamp, episode)
		feedback := isObserverFeedbackLog(content)
		if feedback {
			summary.FeedbackLogs++
		}
		group := attributionGroupFromLog(log)
		constructs := attributionConstructs(content)

		semanticTokens := semanticTokenizer.Tokenize(content)
		semanticKeys := make([]string, len(semanticTokens))
		semanticTypes := make([]string, 0, len(semanticTokens))
		seenTypes := make(map[string]struct{})
		for i, token := range semanticTokens {
			semanticKeys[i] = strconv.Itoa(int(token.Type)) + "\x00" + token.Value
			typeName := token.Type.String()
			if _, ok := seenTypes[typeName]; !ok {
				seenTypes[typeName] = struct{}{}
				semanticTypes = append(semanticTypes, typeName)
			}
		}
		sort.Strings(semanticTypes)
		semanticHash := hashSemanticAttributionTokens(semanticTokens)
		semanticCluster, semanticEmitted := semanticClusterer.process(
			group,
			semanticKeys,
			semanticHash,
			content,
			content,
			semanticTypes,
			constructs,
			timestamp,
			phase,
			feedback,
		)

		compactTokens, _ := logsTokenizer.Tokenize(log.Content)
		compactKeys := make([]string, len(compactTokens))
		for i, token := range compactTokens {
			compactKeys[i] = strconv.Itoa(int(token))
		}
		logsHash := strconv.FormatUint(logpattern.Hash(compactTokens), 16)
		logsCluster, logsEmitted := logsClusterer.process(
			group,
			compactKeys,
			logsHash,
			logpattern.TokensToString(compactTokens),
			content,
			nil,
			constructs,
			timestamp,
			phase,
			feedback,
		)

		key := attributionMappingKey{semantic: semanticCluster.internalID, logs: logsCluster.internalID}
		mapping := mappings[key]
		if mapping == nil {
			mapping = &attributionMapping{
				SemanticClusterID: semanticCluster.ID,
				LogsClusterID:     logsCluster.ID,
				GroupHash:         semanticCluster.GroupHash,
				PhaseCounts:       make(map[string]int),
			}
			mappings[key] = mapping
		}
		mapping.Count++
		mapping.PhaseCounts[phase]++
		if len(mapping.Examples) < 3 && !feedback {
			mapping.Examples = append(mapping.Examples, truncateAttribution(content, 300))
		}
		if semanticEmitted {
			mapping.SemanticEmittedLogs++
		}
		if logsEmitted {
			mapping.LogsEmittedLogs++
		}
		if semanticEmitted && !logsEmitted {
			mapping.SemanticOnlyEmittedLogs++
			if !feedback {
				summary.SemanticOnlyEmittedLogs++
			}
		}
		if logsEmitted && !semanticEmitted && !feedback {
			summary.LogsOnlyEmittedLogs++
		}
		if feedback {
			mapping.FeedbackLogs++
		} else {
			summary.NonFeedbackComparedLogs++
		}
	}

	summary.SemanticClusters = len(semanticClusterer.clusters)
	summary.LogsClusters = len(logsClusterer.clusters)
	for _, cluster := range semanticClusterer.clusters {
		if cluster.Count >= attributionMinEmitCount {
			summary.SemanticEmittedClusters++
		}
	}
	for _, cluster := range logsClusterer.clusters {
		if cluster.Count >= attributionMinEmitCount {
			summary.LogsEmittedClusters++
		}
	}

	mappingList := make([]*attributionMapping, 0, len(mappings))
	for _, mapping := range mappings {
		mappingList = append(mappingList, mapping)
	}
	sort.Slice(mappingList, func(i, j int) bool {
		if mappingList[i].Count != mappingList[j].Count {
			return mappingList[i].Count > mappingList[j].Count
		}
		if mappingList[i].SemanticClusterID != mappingList[j].SemanticClusterID {
			return mappingList[i].SemanticClusterID < mappingList[j].SemanticClusterID
		}
		return mappingList[i].LogsClusterID < mappingList[j].LogsClusterID
	})

	fragmented, overmerged, candidates := summarizeAttributionMappings(
		semanticClusterer.clusters,
		logsClusterer.clusters,
		mappingList,
	)
	if summary.NonFeedbackComparedLogs > 0 {
		summary.SemanticFragmentationRate = float64(fragmented) / float64(summary.NonFeedbackComparedLogs)
		summary.LogsOvermergeRate = float64(overmerged) / float64(summary.NonFeedbackComparedLogs)
	}

	sortClusters := func(clusters []*attributionCluster) {
		sort.Slice(clusters, func(i, j int) bool {
			if clusters[i].NonFeedback != clusters[j].NonFeedback {
				return clusters[i].NonFeedback > clusters[j].NonFeedback
			}
			return clusters[i].ID < clusters[j].ID
		})
	}
	sortClusters(semanticClusterer.clusters)
	sortClusters(logsClusterer.clusters)

	report := patternAttributionReport{
		Scenario:          scenario,
		MatchThreshold:    threshold,
		MinEmitCount:      attributionMinEmitCount,
		Comparison:        "semantic (type, raw value) tokens vs Logs compact tokens; identical fixed-representative positional matcher and ordered input",
		Episode:           episode,
		Summary:           summary,
		SemanticClusters:  semanticClusterer.clusters,
		LogsClusters:      logsClusterer.clusters,
		Mappings:          mappingList,
		CandidateFeatures: candidates,
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding report: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	if err := os.WriteFile(output, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing report: %w", err)
	}
	fmt.Printf("Pattern attribution: %s (%d logs, %d semantic clusters, %d Logs clusters) -> %s\n",
		scenario, len(logs), len(semanticClusterer.clusters), len(logsClusterer.clusters), output)
	return nil
}

func attributionGroupFromLog(log recorderdef.LogData) attributionGroup {
	var group attributionGroup
	for _, tag := range log.Tags {
		key, value, ok := strings.Cut(tag, ":")
		if !ok {
			continue
		}
		switch key {
		case "source":
			group.Source = value
		case "service":
			group.Service = value
		case "env":
			group.Env = value
		case "host":
			group.Host = value
		}
	}
	if group.Host == "" {
		group.Host = log.Hostname
	}
	return group
}

func hashSemanticAttributionTokens(tokens []semanticpatterns.Token) string {
	h := fnv.New64a()
	var buf [8]byte
	for _, token := range tokens {
		binary.LittleEndian.PutUint64(buf[:], uint64(token.Type))
		_, _ = h.Write(buf[:])
		binary.LittleEndian.PutUint64(buf[:], uint64(len(token.Value)))
		_, _ = h.Write(buf[:])
		_, _ = h.Write([]byte(token.Value))
	}
	return strconv.FormatUint(h.Sum64(), 16)
}

func attributionPhase(timestamp int64, episode *EpisodeInfo) string {
	if episode == nil {
		return "unknown"
	}
	phases := []struct {
		name  string
		phase *EpisodePhase
	}{
		{"warmup", episode.Warmup},
		{"baseline", episode.Baseline},
		{"disruption", episode.Disruption},
		{"cooldown", episode.Cooldown},
	}
	for _, item := range phases {
		if item.phase == nil {
			continue
		}
		start, startErr := time.Parse(time.RFC3339Nano, item.phase.Start)
		end, endErr := time.Parse(time.RFC3339Nano, item.phase.End)
		if startErr == nil && endErr == nil && timestamp >= start.Unix() && timestamp < end.Unix() {
			return item.name
		}
	}
	return "outside"
}

func attributionConstructs(content string) []string {
	features := make([]string, 0, 7)
	trimmed := strings.TrimSpace(content)
	if json.Valid([]byte(trimmed)) {
		features = append(features, "json")
	}
	if attributionUUIDPattern.MatchString(content) {
		features = append(features, "uuid")
	}
	if attributionHexPattern.MatchString(content) {
		features = append(features, "long_hex")
	}
	if hasLongDigitRun(content, 12) {
		features = append(features, "long_integer")
	}
	if strings.Contains(content, "\x1b[") || strings.Contains(content, `\u001b[`) {
		features = append(features, "ansi_escape")
	}
	if strings.Contains(content, "://") {
		features = append(features, "uri")
	}
	if strings.Contains(content, "=") && (strings.Contains(content, " ") || strings.Contains(content, ",")) {
		features = append(features, "key_value")
	}
	return features
}

func hasLongDigitRun(content string, minimum int) bool {
	run := 0
	for i := 0; i < len(content); i++ {
		if content[i] >= '0' && content[i] <= '9' {
			run++
			if run >= minimum {
				return true
			}
		} else {
			run = 0
		}
	}
	return false
}

func isObserverFeedbackLog(content string) bool {
	return strings.Contains(content, "[observer]") ||
		strings.Contains(content, "Correlated behavior change detected:") ||
		strings.Contains(content, "Log pattern change rate detected:")
}

func truncateAttribution(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func summarizeAttributionMappings(
	semanticClusters []*attributionCluster,
	logsClusters []*attributionCluster,
	mappings []*attributionMapping,
) (fragmentedLogs int, overmergedLogs int, candidates []attributionCandidate) {
	semanticByID := make(map[string]*attributionCluster, len(semanticClusters))
	logsByID := make(map[string]*attributionCluster, len(logsClusters))
	semanticEdges := make(map[string][]*attributionMapping)
	logsEdges := make(map[string][]*attributionMapping)
	for _, cluster := range semanticClusters {
		semanticByID[cluster.ID] = cluster
	}
	for _, cluster := range logsClusters {
		logsByID[cluster.ID] = cluster
	}
	for _, mapping := range mappings {
		if mapping.Count == mapping.FeedbackLogs {
			continue
		}
		semanticEdges[mapping.SemanticClusterID] = append(semanticEdges[mapping.SemanticClusterID], mapping)
		logsEdges[mapping.LogsClusterID] = append(logsEdges[mapping.LogsClusterID], mapping)
	}

	type candidateAccumulator struct {
		clusters map[string]struct{}
		logs     int
		fragment int
		emitGap  int
		kind     string
	}
	accumulators := make(map[string]*candidateAccumulator)
	addCandidate := func(name, kind, clusterID string, logs, fragment, emitGap int) {
		key := kind + ":" + name
		acc := accumulators[key]
		if acc == nil {
			acc = &candidateAccumulator{clusters: make(map[string]struct{}), kind: kind}
			accumulators[key] = acc
		}
		acc.clusters[clusterID] = struct{}{}
		acc.logs += logs
		acc.fragment += fragment
		acc.emitGap += emitGap
	}

	for semanticID, edges := range semanticEdges {
		total, largest, emitGap := 0, 0, 0
		for _, edge := range edges {
			count := edge.Count - edge.FeedbackLogs
			total += count
			largest = max(largest, count)
			emitGap += edge.SemanticOnlyEmittedLogs
		}
		fragment := total - largest
		fragmentedLogs += fragment
		cluster := semanticByID[semanticID]
		if cluster == nil || (fragment == 0 && emitGap == 0) {
			continue
		}
		for _, tokenType := range cluster.TokenTypes {
			switch tokenType {
			case "Word", "Whitespace", "SpecialCharacter", "NumericValue":
				continue
			}
			addCandidate(tokenType, "semantic_token_type", semanticID, total, fragment, emitGap)
		}
		for _, construct := range cluster.Constructs {
			addCandidate(construct, "raw_construct", semanticID, total, fragment, emitGap)
		}
	}

	for _, edges := range logsEdges {
		total, largest := 0, 0
		for _, edge := range edges {
			count := edge.Count - edge.FeedbackLogs
			total += count
			largest = max(largest, count)
		}
		overmergedLogs += total - largest
	}

	for key, acc := range accumulators {
		name := strings.TrimPrefix(key, acc.kind+":")
		candidates = append(candidates, attributionCandidate{
			Name:                    name,
			Kind:                    acc.kind,
			Clusters:                len(acc.clusters),
			Logs:                    acc.logs,
			FragmentedLogs:          acc.fragment,
			SemanticOnlyEmittedLogs: acc.emitGap,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].SemanticOnlyEmittedLogs != candidates[j].SemanticOnlyEmittedLogs {
			return candidates[i].SemanticOnlyEmittedLogs > candidates[j].SemanticOnlyEmittedLogs
		}
		if candidates[i].FragmentedLogs != candidates[j].FragmentedLogs {
			return candidates[i].FragmentedLogs > candidates[j].FragmentedLogs
		}
		return candidates[i].Name < candidates[j].Name
	})
	return fragmentedLogs, overmergedLogs, candidates
}
