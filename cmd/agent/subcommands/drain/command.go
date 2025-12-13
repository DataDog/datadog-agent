// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package drain implements 'agent drain'.
package drain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.uber.org/fx"

	"github.com/faceair/drain"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// CliParams holds the command-line arguments for the drain subcommand.
type CliParams struct {
	*command.GlobalParams

	// InputFilePath represents the path to the input log file.
	InputFilePath string

	// Threshold represents the cluster size threshold for filtering logs.
	Threshold int

	// ScoreThreshold represents the score threshold for filtering logs (if set, overrides Threshold).
	ScoreThreshold *float64

	// ProgressiveTraining indicates whether to train the drain processor on all logs before filtering.
	ProgressiveTraining bool

	// LogClusterDepth represents the depth of the log cluster tree.
	LogClusterDepth int

	// SimTh represents the similarity threshold for clustering.
	SimTh float64

	// MaxChildren represents the maximum number of children in the cluster tree.
	MaxChildren int

	// PrintInfo indicates whether to print detailed information (score, size, etc.) for each line.
	PrintInfo bool

	// TopClusters represents the number of top clusters to display.
	TopClusters int

	// HideOutput indicates whether to hide the filtered log lines output.
	HideOutput bool

	// OrderByScore indicates whether to order clusters by score instead of size.
	OrderByScore bool

	// AIMark indicates whether to mark clusters with AI (identify important clusters).
	AIMark bool

	// AISmartPatterns indicates whether to generate smart patterns with AI (annotate pattern variables).
	AISmartPatterns bool

	// TokenizeUsingSpace indicates whether to use simple space-based tokenization.
	TokenizeUsingSpace bool

	// TokenDelimiters represents the delimiters used for tokenization.
	TokenDelimiters string

	// TokenDelimitersMerge indicates whether to merge delimiters with tokens.
	TokenDelimitersMerge bool

	// FirstOccurrences indicates whether to display first occurrences for each cluster.
	FirstOccurrences bool
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &CliParams{
		GlobalParams: globalParams,
	}

	var scoreThreshold float64
	cmd := &cobra.Command{
		Use:   "drain [filepath]",
		Short: "Filter logs using drain processor",
		Long:  `Read logs from a file, apply drain filtering, and write filtered results to stdout`,
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cliParams.InputFilePath = args[0]
			// Set ScoreThreshold pointer if flag was provided
			if scoreThreshold > 0 {
				cliParams.ScoreThreshold = &scoreThreshold
			}
			bundleParams := command.GetDefaultCoreBundleParams(cliParams.GlobalParams)
			// Enable logging at info level
			bundleParams.LogParams = log.ForOneShot(command.LoggerName, "info", true)
			return fxutil.OneShot(runDrain,
				fx.Supply(cliParams),
				fx.Supply(bundleParams),
				core.Bundle(),
				secretsnoopfx.Module(),
			)
		},
	}
	cmd.Flags().IntVarP(&cliParams.Threshold, "threshold", "t", 10, "Cluster size threshold for filtering logs (default: 10)")
	cmd.Flags().Float64Var(&scoreThreshold, "score-threshold", 0, "Score threshold for filtering logs (if set, overrides threshold)")
	cmd.Flags().BoolVarP(&cliParams.ProgressiveTraining, "progressive-training", "", false, "Train the drain processor progressively on logs before filtering")
	cmd.Flags().IntVar(&cliParams.LogClusterDepth, "log-cluster-depth", 4, "Depth of the log cluster tree (default: 4)")
	cmd.Flags().Float64Var(&cliParams.SimTh, "sim-th", 0.4, "Similarity threshold for clustering (default: 0.4)")
	cmd.Flags().IntVar(&cliParams.MaxChildren, "max-children", 100, "Maximum number of children in the cluster tree (default: 100)")
	cmd.Flags().BoolVarP(&cliParams.PrintInfo, "print-info", "", false, "Print detailed information (score, size, etc.) for each line")
	cmd.Flags().IntVar(&cliParams.TopClusters, "top-clusters", 10, "Number of top clusters to display (default: 10)")
	cmd.Flags().BoolVarP(&cliParams.HideOutput, "hide-output", "", false, "Hide filtered log lines output (only show summary statistics)")
	cmd.Flags().BoolVarP(&cliParams.OrderByScore, "order-by-score", "", false, "Order clusters by score instead of size")
	cmd.Flags().BoolVar(&cliParams.AIMark, "ai-mark", false, "Use AI to mark important clusters (default: false)")
	cmd.Flags().BoolVar(&cliParams.AISmartPatterns, "ai-smart-patterns", false, "Use AI to generate smart patterns with annotated variables (default: false)")
	cmd.Flags().BoolVar(&cliParams.TokenizeUsingSpace, "tokenize-using-space", false, "Use simple space-based tokenization (default: false)")
	cmd.Flags().StringVar(&cliParams.TokenDelimiters, "token-delimiters", "[](){},;.", "Delimiters used for tokenization (default: \"[](){},;.\")")
	cmd.Flags().BoolVar(&cliParams.TokenDelimitersMerge, "token-delimiters-merge", false, "Merge delimiters with tokens instead of splitting on them (default: false)")
	cmd.Flags().BoolVar(&cliParams.FirstOccurrences, "first-occurrences", false, "Display first occurrence for each cluster (default: false)")

	return []*cobra.Command{cmd}
}

// chatCompletionRequest represents the request body for the AI completion API
type chatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
}

// message represents a single message in the chat completion request
type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionResponse represents the response from the AI completion API
type chatCompletionResponse struct {
	Choices []choice `json:"choices"`
}

// choice represents a choice in the completion response
type choice struct {
	Message message `json:"message"`
}

func aiCompletion(question string) (string, error) {
	// Get auth token from ddtool
	cmd := exec.Command("ddtool", "auth", "token", "rapid-ai-platform", "--datacenter", "us1.staging.dog", "--http-header")
	authOutput, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get auth token: %w", err)
	}

	// Parse the auth header (format: "Authorization: Bearer <token>")
	authHeader := strings.TrimSpace(string(authOutput))
	if !strings.HasPrefix(authHeader, "Authorization: ") {
		return "", fmt.Errorf("unexpected auth header format: %s", authHeader)
	}
	authToken := strings.TrimPrefix(authHeader, "Authorization: ")

	// Prepare request body
	reqBody := chatCompletionRequest{
		Model: "openai/gpt-5-mini",
		Messages: []message{
			{
				Role:    "user",
				Content: question,
			},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(context.Background(), "POST", "https://ai-gateway.us1.staging.dog/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("source", "datadog-agent")
	req.Header.Set("org-id", "2")
	req.Header.Set("Authorization", authToken)

	// Make HTTP request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for HTTP errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var completionResp chatCompletionResponse
	if err := json.Unmarshal(respBody, &completionResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Extract completion text
	if len(completionResp.Choices) == 0 {
		return "", errors.New("no choices in response")
	}

	return completionResp.Choices[0].Message.Content, nil
}

// runDrain reads the input file, applies drain filtering, and writes filtered logs to stdout.
func runDrain(lc log.Component, _ config.Component, cliParams *CliParams) error {
	if cliParams.ScoreThreshold != nil && cliParams.ProgressiveTraining {
		lc.Warn("Score threshold is set and progressive training is enabled. The score is not accurate in this mode.")
	}
	if cliParams.ProgressiveTraining && (cliParams.AIMark || cliParams.AISmartPatterns) {
		lc.Warn("Progressive training is enabled and AI processing is enabled. The AI processing will not work in this mode.")
	}
	fmt.Println("--------------------------------")

	// Configure tokenization based on CLI parameters
	processor.SetTokenizationConfig(
		cliParams.TokenizeUsingSpace,
		cliParams.TokenDelimiters,
		cliParams.TokenDelimitersMerge,
	)

	// Create drain processor
	drainProcessor := processor.NewDrainProcessor("drain-command", &drain.Config{
		LogClusterDepth: cliParams.LogClusterDepth,
		SimTh:           cliParams.SimTh,
		MaxChildren:     cliParams.MaxChildren,
		ParamString:     "<*>",
	})

	// Read entire file and split by line feeds
	fileContent, err := os.ReadFile(cliParams.InputFilePath)
	if err != nil {
		return fmt.Errorf("error reading input file %s: %w", cliParams.InputFilePath, err)
	}

	// Split by line feeds
	lines := bytes.Split(fileContent, []byte("\n"))
	processedCount := 0
	filteredCount := 0

	// These clusters are important according to AI
	aiMarkedClusters := make(map[int]bool)
	// Pattern names
	aiSmartPatterns := make(map[int]string)
	if !cliParams.ProgressiveTraining {
		// Train first
		trainStart := time.Now()
		for _, line := range lines {
			tokens := processor.DrainTokenize(line)
			// drainProcessor.Train(tokens)
			drainProcessor.MatchAndTrain(string(line), tokens, "log.txt")
		}
		trainDuration := time.Since(trainStart)
		lc.Infof("[profile] Train duration: %s (%.0fK logs/s)", trainDuration, float64(len(lines))/trainDuration.Seconds()/1000)

		if cliParams.AIMark || cliParams.AISmartPatterns {
			aiClusters := drainProcessor.Clusters()
			slices.SortFunc(aiClusters, func(a, b *drain.LogCluster) int {
				return b.Size() - a.Size()
			})

			// --- Mark clusters ---
			if cliParams.AIMark {
				clusterPrompt := strings.Builder{}
				clusterPrompt.WriteString(`
You will have multiple log patterns. Your goal is to determine which pattern we want to monitor. These patterns are logs that have warnings / errors / meaningful information to improve remediation time.
We want to keep only the patterns that match these conditions.

You should output only the pattern IDs separated by commas. No other text. You can reply nothing if no pattern matches these conditions.
---`)
				for i, cluster := range aiClusters {
					clusterStr := cluster.String()
					if len(clusterStr) > 160 {
						clusterStr = clusterStr[:160] + "..."
					}
					clusterPrompt.WriteString(fmt.Sprintf("#%d: %s\n", cluster.ID(), clusterStr))
					if i >= cliParams.TopClusters {
						break
					}
				}

				prompt := clusterPrompt.String()
				aiCompletionStart := time.Now()
				aiResponse, err := aiCompletion(prompt)
				if err != nil {
					return fmt.Errorf("failed to process clusters with AI: prompt=%s, error=%w", prompt, err)
				}
				aiCompletionDuration := time.Since(aiCompletionStart)
				lc.Infof("[profile] AI completion (mark clusters) duration: %s (%.0f clusters/s)", aiCompletionDuration, float64(len(aiClusters))/aiCompletionDuration.Seconds())

				aiMarkedClusterIDs := strings.Split(aiResponse, ",")
				lc.Infof("AI marked clusters: %v", aiMarkedClusterIDs)
				for _, clusterID := range aiMarkedClusterIDs {
					id, err := strconv.Atoi(clusterID)
					if err != nil {
						return fmt.Errorf("failed to convert cluster ID to int: clusterID=%s, error=%w", clusterID, err)
					}
					aiMarkedClusters[id] = true
				}
			}

			// --- Smart patterns ---
			if cliParams.AISmartPatterns {
				smartPatternsPrompt := strings.Builder{}
				smartPatternsPrompt.WriteString(`
You will have multiple log patterns. Your goal is to annotate each pattern variable. The variables are the <*> tokens, you must replace them by their name. For example, Request <*> could be Request <http_status>.
You will have the first log occurrence after the pattern to guide you.
Each line is a pattern. You should output only the patterns in order with the new variables. No other text.
---
`)
				for i, cluster := range aiClusters {
					template := cluster.GetTemplate()
					if len(template) > 160 {
						template = template[:160] + "..."
					}
					firstOcc := drainProcessor.GetClusterInfo(cluster.ID()).FirstOccurrence
					if len(firstOcc) > 160 {
						firstOcc = firstOcc[:160] + "..."
					}
					smartPatternsPrompt.WriteString(fmt.Sprintf("%s | First occurrence: %s\n", template, firstOcc))
					if i >= cliParams.TopClusters {
						break
					}
				}

				prompt := smartPatternsPrompt.String()
				aiCompletionStart := time.Now()
				aiResponse, err := aiCompletion(prompt)
				if err != nil {
					return fmt.Errorf("failed to process smart patterns with AI: prompt=%s, error=%w", prompt, err)
				}
				aiCompletionDuration := time.Since(aiCompletionStart)
				lc.Infof("[profile] AI completion (smart patterns) duration: %s (%.0f patterns/s)", aiCompletionDuration, float64(len(aiClusters))/aiCompletionDuration.Seconds())

				aiSmartPatternsList := strings.Split(aiResponse, "\n")
				for i, pattern := range aiSmartPatternsList {
					aiSmartPatterns[aiClusters[i].ID()] = pattern
				}
			}
		}
	}

	// If trained first, we can compute accurate stats about cluster distribution
	clusters := drainProcessor.Clusters()
	slices.SortFunc(clusters, func(a, b *drain.LogCluster) int {
		return a.Size() - b.Size()
	})
	totalSize := 0.0
	for _, cluster := range clusters {
		totalSize += float64(cluster.Size())
	}

	seenLines := make(map[string]bool)
	orderByScoreData := []struct {
		Score float64
		Line  string
	}{}

	// Inference
	for _, line := range lines {
		tokens := processor.DrainTokenize(line)
		cluster := drainProcessor.Match(tokens)
		if cliParams.ProgressiveTraining {
			drainProcessor.Train(tokens)
		}
		s := 0
		if cluster != nil {
			s = cluster.Size()
		}

		// The score is ~how many logs are less similar than this log
		upperBound := 0
		for _, cluster := range clusters {
			if s < cluster.Size() {
				break
			}
			upperBound++
		}
		score := float64(upperBound) / float64(len(clusters))
		if cliParams.OrderByScore {
			if !seenLines[string(line)] {
				orderByScoreData = append(orderByScoreData, struct {
					Score float64
					Line  string
				}{
					Score: score,
					Line:  string(line),
				})
			}
		}
		seenLines[string(line)] = true

		// Filter by score threshold if set, otherwise use size threshold
		var toIgnore bool
		if cliParams.ScoreThreshold != nil && *cliParams.ScoreThreshold > 0 {
			toIgnore = score >= *cliParams.ScoreThreshold
		} else {
			toIgnore = s >= cliParams.Threshold
		}

		// If this cluster is marked as important, don't ignore it
		if cliParams.AIMark {
			_, isMarked := aiMarkedClusters[cluster.ID()]
			toIgnore = toIgnore && !isMarked
		}

		// Write non-filtered lines to stdout
		if toIgnore {
			filteredCount++
		} else {
			processedCount++
			if !cliParams.HideOutput {
				lineStr := string(line)
				if len(lineStr) > 130 {
					lineStr = lineStr[:130] + "..."
				}
				if cliParams.PrintInfo {
					fmt.Printf("%s: score=%f, s=%d, totalSize=%f, marked=%v\n", lineStr, score, s, totalSize, aiMarkedClusters[cluster.ID()])
				} else if !cliParams.OrderByScore {
					fmt.Println(lineStr)
				}
			}
		}
	}

	if cliParams.OrderByScore {
		slices.SortFunc(orderByScoreData, func(a, b struct {
			Score float64
			Line  string
		}) int {
			if a.Score < b.Score {
				return -1
			}
			return 1
		})
		for _, data := range orderByScoreData {
			fmt.Printf("%3.3f | %s\n", data.Score, data.Line)
		}
	}

	fmt.Println("--------------------------------")
	slices.SortFunc(clusters, func(a, b *drain.LogCluster) int {
		return b.Size() - a.Size()
	})
	fmt.Printf("%d total clusters\n", len(clusters))
	fmt.Printf("Top %d clusters:\n", cliParams.TopClusters)
	for i, cluster := range clusters {
		if i >= cliParams.TopClusters {
			break
		}
		clusterStr := cluster.GetTemplate()
		if cliParams.AISmartPatterns {
			pattern, exists := aiSmartPatterns[cluster.ID()]
			if exists {
				clusterStr = pattern
			}
		}
		marked := ""
		if cliParams.AIMark {
			marked = fmt.Sprintf(" (marked=%v)", aiMarkedClusters[cluster.ID()])
		}
		firstOcc := ""
		if cliParams.FirstOccurrences {
			firstOcc = fmt.Sprintf(" | First occurrence: %s", drainProcessor.GetClusterInfo(cluster.ID()).FirstOccurrence)
		}
		fmt.Printf("Cluster %d: size: %d %s: %s%s\n", i+1, cluster.Size(), marked, clusterStr, firstOcc)
	}

	fmt.Printf("Processed %d lines: filtered %f%%\n", len(lines), float64(filteredCount)/float64(len(lines))*100)

	return nil
}
