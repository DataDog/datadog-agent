// main.go - Tests load generator statistics to ensure tests are succeeding
// Validates that the load generator has tested application endpoints properly
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultTimeout      = 2 * time.Minute
	pollInterval        = 5 * time.Second
	defaultLoadGenPort  = "8089"
	minExpectedRequests = 2
)

// Config holds the test configuration
type Config struct {
	LoadGenPort    string
	Timeout        time.Duration
	MinRequests    int
	MinEndpoints   int
	RequestTimeout time.Duration
}

// StatsResponse represents the /stats/requests endpoint response
type StatsResponse struct {
	Stats  []EndpointStat `json:"stats"`
	Errors []ErrorStat    `json:"errors"`
}

// EndpointStat represents statistics for a single endpoint
type EndpointStat struct {
	Name           string  `json:"name"`
	Method         string  `json:"method"`
	NumRequests    int     `json:"num_requests"`
	NumFailures    int     `json:"num_failures"`
	MedianRespTime float64 `json:"median_response_time"`
	AvgRespTime    float64 `json:"avg_response_time"`
	MinRespTime    float64 `json:"min_response_time"`
	MaxRespTime    float64 `json:"max_response_time"`
}

// ErrorStat represents an error reported by the load generator
type ErrorStat struct {
	Method      string `json:"method"`
	Name        string `json:"name"`
	Occurrences int    `json:"occurrences"`
	Error       string `json:"error"`
}

// ExceptionsResponse represents the /exceptions endpoint response
type ExceptionsResponse struct {
	Exceptions []ExceptionStat `json:"exceptions"`
}

// ExceptionStat represents an exception reported by the load generator
type ExceptionStat struct {
	Count     int    `json:"count"`
	Msg       string `json:"msg"`
	Traceback string `json:"traceback"`
}

// ValidationResult holds the result of validating load generator stats
type ValidationResult struct {
	Success       bool
	Error         string
	Stats         *StatsResponse
	Exceptions    *ExceptionsResponse
	ErrorDetails  []string
	RealEndpoints []EndpointStat
	TotalRequests int
}

// Colors for terminal output
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[0;32m"
	colorBlue   = "\033[0;34m"
	colorYellow = "\033[1;33m"
	colorRed    = "\033[0;31m"
)

func main() {
	cfg := loadConfig()

	fmt.Println("🧪 Testing load-generator component - Stats Validation")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	printConfig(cfg)

	result, err := waitForValidStats(cfg)
	if err != nil {
		printError(fmt.Sprintf("Failed to validate load generator: %v", err))
		os.Exit(1)
	}

	if !result.Success {
		printError(result.Error)
		if len(result.ErrorDetails) > 0 {
			fmt.Println()
			for _, detail := range result.ErrorDetails {
				fmt.Println(detail)
			}
		}
		os.Exit(1)
	}

	printStats(result)
	printSuccess("Load generator validation passed! ✅")
}

func loadConfig() Config {
	timeout := defaultTimeout
	if t := os.Getenv("STATS_TIMEOUT"); t != "" {
		if d, err := time.ParseDuration(t); err == nil {
			timeout = d
		}
	}

	minRequests := minExpectedRequests
	if r := os.Getenv("MIN_REQUESTS"); r != "" {
		if _, err := fmt.Sscanf(r, "%d", &minRequests); err != nil {
			minRequests = minExpectedRequests
		}
	}

	minEndpoints := 1
	if e := os.Getenv("MIN_ENDPOINTS"); e != "" {
		if _, err := fmt.Sscanf(e, "%d", &minEndpoints); err != nil {
			minEndpoints = 1
		}
	}

	return Config{
		LoadGenPort:    getEnvOrDefault("LOAD_GEN_PORT", defaultLoadGenPort),
		Timeout:        timeout,
		MinRequests:    minRequests,
		MinEndpoints:   minEndpoints,
		RequestTimeout: 5 * time.Second,
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func waitForValidStats(cfg Config) (*ValidationResult, error) {
	printBlue(fmt.Sprintf("Waiting for load generator stats (min %d requests, min %d endpoints)", cfg.MinRequests, cfg.MinEndpoints))
	printBlue(fmt.Sprintf("Timeout: %v, Poll interval: %v", cfg.Timeout, pollInterval))
	fmt.Println()

	deadline := time.Now().Add(cfg.Timeout)
	attempt := 0

	var lastResult *ValidationResult

	for time.Now().Before(deadline) {
		attempt++
		remaining := time.Until(deadline).Round(time.Second)

		fmt.Printf("\r%s⏳ Attempt %d - Checking load generator stats... (timeout in %v)%s",
			colorBlue, attempt, remaining, colorReset)

		result := validateLoadGeneratorStats(cfg)
		lastResult = result

		if result.Success {
			fmt.Println() // New line after progress
			fmt.Println()
			printSuccess(fmt.Sprintf("Validation passed with %d requests across %d endpoint(s)",
				result.TotalRequests, len(result.RealEndpoints)))
			return result, nil
		}

		// Check if we should keep waiting or fail immediately
		// If load generator is not reachable, keep waiting
		// If validation logic fails, we might still want to wait for more requests
		if result.Stats != nil && result.TotalRequests >= cfg.MinRequests {
			// We have enough requests but validation failed - this is a real failure
			fmt.Println()
			return result, nil
		}

		time.Sleep(pollInterval)
	}

	fmt.Println() // New line after progress
	if lastResult != nil {
		return lastResult, nil
	}
	return &ValidationResult{
		Success: false,
		Error:   fmt.Sprintf("timeout after %v: could not validate load generator stats", cfg.Timeout),
	}, nil
}

func validateLoadGeneratorStats(cfg Config) *ValidationResult {
	result := &ValidationResult{
		Success:      false,
		ErrorDetails: []string{},
	}

	// Fetch /stats/requests
	statsURL := fmt.Sprintf("http://127.0.0.1:%s/stats/requests", cfg.LoadGenPort)
	stats, err := fetchStats(statsURL, cfg.RequestTimeout)
	if err != nil {
		result.Error = fmt.Sprintf("Could not fetch load generator stats: %v", err)
		return result
	}
	result.Stats = stats

	// Collect error information (errors are okay as long as endpoints exist)
	if len(stats.Errors) > 0 {
		result.ErrorDetails = append(result.ErrorDetails, "Load generator reported errors:")
		for _, errStat := range stats.Errors {
			result.ErrorDetails = append(result.ErrorDetails,
				fmt.Sprintf("  - %s %s: %d failures - %s", errStat.Method, errStat.Name, errStat.Occurrences, errStat.Error))
		}
	}

	// Check that stats exist and have endpoints
	if len(stats.Stats) == 0 {
		result.Error = "No endpoint stats found in load generator response."
		if len(result.ErrorDetails) > 0 {
			result.Error += "\n\n" + strings.Join(result.ErrorDetails, "\n")
		}
		return result
	}

	// Filter out "Aggregated" stats to count real endpoints
	realEndpoints := []EndpointStat{}
	for _, s := range stats.Stats {
		if s.Name != "Aggregated" {
			realEndpoints = append(realEndpoints, s)
		}
	}
	result.RealEndpoints = realEndpoints

	// Calculate total requests
	totalRequests := 0
	for _, s := range realEndpoints {
		totalRequests += s.NumRequests
	}
	result.TotalRequests = totalRequests

	// Check for single request / health check only case
	singleEndpointIssue := len(realEndpoints) == 1 && totalRequests <= 1

	// Check each stat to ensure endpoints report failures (indicating they were tested)
	var failedEndpoints []string
	var endpointsWithNoSuccesses []string

	for _, stat := range stats.Stats {
		// Skip aggregated stats
		if stat.Name == "Aggregated" {
			continue
		}

		endpointName := stat.Name
		if stat.Method != "" {
			endpointName = fmt.Sprintf("%s %s", stat.Method, stat.Name)
		}

		// Endpoint must have been tested (have requests)
		if stat.NumRequests == 0 {
			failedEndpoints = append(failedEndpoints, fmt.Sprintf("  - %s: No requests reported", endpointName))
		} else {
			successfulRequests := stat.NumRequests - stat.NumFailures
			// Ignore failures if only 1 request (might be a specific ID that doesn't exist)
			if successfulRequests <= 0 && stat.NumRequests > 1 {
				endpointsWithNoSuccesses = append(endpointsWithNoSuccesses,
					fmt.Sprintf("  - %s: %d total, %d failures, 0 successes", endpointName, stat.NumRequests, stat.NumFailures))
			}
		}
	}

	// Fetch /exceptions endpoint and append all exceptions if present
	exceptionsURL := fmt.Sprintf("http://127.0.0.1:%s/exceptions", cfg.LoadGenPort)
	exceptions, err := fetchExceptions(exceptionsURL, cfg.RequestTimeout)
	if err == nil && exceptions != nil && len(exceptions.Exceptions) > 0 {
		result.Exceptions = exceptions
		result.ErrorDetails = append(result.ErrorDetails, "\nLoad generator exceptions:")
		for _, exc := range exceptions.Exceptions {
			result.ErrorDetails = append(result.ErrorDetails, fmt.Sprintf("  - [%dx] %s", exc.Count, exc.Msg))
			if exc.Traceback != "" {
				for _, line := range strings.Split(strings.TrimSpace(exc.Traceback), "\n") {
					result.ErrorDetails = append(result.ErrorDetails, "    "+line)
				}
			}
		}
	}

	// Check for single endpoint issue (deferred to include exceptions in error)
	if singleEndpointIssue {
		endpointName := "unknown"
		if len(realEndpoints) > 0 {
			endpointName = realEndpoints[0].Name
		}
		result.Error = fmt.Sprintf(`Load generator only has a single request to '%s'. This appears to be just the base health check, not actual load testing.

To fix this, the locustfile.py needs to:
1. Implement task methods that test the application's actual API endpoints (e.g., CRUD operations, business logic flows)
2. Update the task_map dictionary in dispatch_task() to include your custom tasks
3. Configure task weights via LOAD_BASELINE_TASK_WEIGHTS and LOAD_INTERVENTION_TASK_WEIGHTS environment variables in docker-compose.yml.`, endpointName)
		if len(result.ErrorDetails) > 0 {
			result.Error += "\n\n" + strings.Join(result.ErrorDetails, "\n")
		}
		return result
	}

	// Build error message if validation fails
	var validationErrors []string
	if len(failedEndpoints) > 0 {
		validationErrors = append(validationErrors, "Endpoints with no requests reported:")
		validationErrors = append(validationErrors, failedEndpoints...)
	}

	if len(endpointsWithNoSuccesses) > 0 {
		validationErrors = append(validationErrors, "\nEndpoints with no successful requests:")
		validationErrors = append(validationErrors, endpointsWithNoSuccesses...)
	}

	if len(validationErrors) > 0 {
		result.Error = "Load generator validation failed:\n" + strings.Join(validationErrors, "\n")
		if len(result.ErrorDetails) > 0 {
			result.Error += "\n\n" + strings.Join(result.ErrorDetails, "\n")
		}
		return result
	}

	// Check minimum requirements
	if totalRequests < cfg.MinRequests {
		result.Error = fmt.Sprintf("Not enough requests yet: %d (need at least %d)", totalRequests, cfg.MinRequests)
		return result
	}

	if len(realEndpoints) < cfg.MinEndpoints {
		result.Error = fmt.Sprintf("Not enough endpoints tested: %d (need at least %d)", len(realEndpoints), cfg.MinEndpoints)
		return result
	}

	// All checks passed
	result.Success = true
	if len(result.ErrorDetails) > 0 {
		result.Error = strings.Join(result.ErrorDetails, "\n")
	}
	return result
}

func fetchStats(url string, timeout time.Duration) (*StatsResponse, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var stats StatsResponse
	if err := json.Unmarshal(body, &stats); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return &stats, nil
}

func fetchExceptions(url string, timeout time.Duration) (*ExceptionsResponse, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var exceptions ExceptionsResponse
	if err := json.Unmarshal(body, &exceptions); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return &exceptions, nil
}

func printStats(result *ValidationResult) {
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	printBlue("Endpoint Statistics:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	for _, stat := range result.RealEndpoints {
		fmt.Println()
		endpointName := stat.Name
		if stat.Method != "" {
			endpointName = fmt.Sprintf("%s %s", stat.Method, stat.Name)
		}
		fmt.Printf("Endpoint: %s\n", endpointName)
		fmt.Printf("  Requests: %d\n", stat.NumRequests)
		fmt.Printf("  Failures: %d\n", stat.NumFailures)
		successRate := 0.0
		if stat.NumRequests > 0 {
			successRate = float64(stat.NumRequests-stat.NumFailures) / float64(stat.NumRequests) * 100
		}
		fmt.Printf("  Success Rate: %s%.1f%%%s\n", getSuccessRateColor(successRate), successRate, colorReset)
		fmt.Printf("  Response Time: avg=%.1fms, min=%.1fms, max=%.1fms\n",
			stat.AvgRespTime, stat.MinRespTime, stat.MaxRespTime)
	}

	// Print summary
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	printBlue("Summary:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  Total Endpoints: %d\n", len(result.RealEndpoints))
	fmt.Printf("  Total Requests: %d\n", result.TotalRequests)

	totalFailures := 0
	for _, stat := range result.RealEndpoints {
		totalFailures += stat.NumFailures
	}
	fmt.Printf("  Total Failures: %d\n", totalFailures)

	if result.TotalRequests > 0 {
		overallSuccessRate := float64(result.TotalRequests-totalFailures) / float64(result.TotalRequests) * 100
		fmt.Printf("  Overall Success Rate: %s%.1f%%%s\n",
			getSuccessRateColor(overallSuccessRate), overallSuccessRate, colorReset)
	}

	// Print error details if any
	if len(result.ErrorDetails) > 0 {
		fmt.Println()
		printWarning("Warnings/Errors (non-fatal):")
		for _, detail := range result.ErrorDetails {
			fmt.Println(detail)
		}
	}

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

func getSuccessRateColor(rate float64) string {
	if rate >= 95 {
		return colorGreen
	} else if rate >= 80 {
		return colorYellow
	}
	return colorRed
}

func printConfig(cfg Config) {
	printBlue("Configuration:")
	fmt.Printf("  Load Gen Port: %s\n", cfg.LoadGenPort)
	fmt.Printf("  Timeout: %v\n", cfg.Timeout)
	fmt.Printf("  Min Requests: %d\n", cfg.MinRequests)
	fmt.Printf("  Min Endpoints: %d\n", cfg.MinEndpoints)
	fmt.Println()
}

func printSuccess(msg string) {
	fmt.Printf("%s✅ %s%s\n", colorGreen, msg, colorReset)
}

func printWarning(msg string) {
	fmt.Printf("%s⚠️  %s%s\n", colorYellow, msg, colorReset)
}

func printError(msg string) {
	fmt.Printf("%s❌ %s%s\n", colorRed, msg, colorReset)
}

func printBlue(msg string) {
	fmt.Printf("%s%s%s\n", colorBlue, msg, colorReset)
}
