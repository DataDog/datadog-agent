// image-promotion is a CLI tool for publishing container images via a gateway service.
//
// It builds a typed JSON payload from flags or environment variables, POSTs to
// the gateway, and polls for completion.
//
// Usage:
//
//	bazel run //tools/image-promotion -- \
//	  --gateway-url "$ARTIFACT_GATEWAY_URL" \
//	  --sources "src-registry/image:tag-amd64,src-registry/image:tag-arm64" \
//	  --destinations "image:tag" \
//	  --registries "public"
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// request is the typed payload sent to the gateway.
type request struct {
	Sources       []string          `json:"sources,omitempty"`
	Destinations  []string          `json:"destinations,omitempty"`
	TagReference  string            `json:"tag_reference,omitempty"`
	NewTags       []string          `json:"new_tags,omitempty"`
	Registries    []string          `json:"registries"`
	Variables     map[string]string `json:"variables,omitempty"`
	MergeStrategy string            `json:"merge_strategy,omitempty"`
	Signing       *bool             `json:"signing,omitempty"`
}

// response is returned by the gateway for both POST and GET.
type response struct {
	ID     string `json:"id"`
	WebURL string `json:"web_url"`
	Status string `json:"status"`
}

// config holds parsed CLI flags.
type config struct {
	Sources       string
	Destinations  string
	Registries    string
	TagReference  string
	NewTags       string
	Variables     string
	MergeStrategy string
	Signing       string
	GatewayURL    string
	Token         string
	Timeout       int
	PollInterval  int
}

func main() {
	if err := run(os.Args[1:], http.DefaultClient); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, httpClient *http.Client) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}

	req, err := buildRequest(cfg)
	if err != nil {
		return err
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	fmt.Printf("Payload: %s\n", body)

	resp, err := doPost(httpClient, cfg.GatewayURL+"/v1/image-publications", cfg.Token, body)
	if err != nil {
		return err
	}

	fmt.Printf("Publication triggered: %s — %s\n", resp.ID, resp.WebURL)

	return poll(httpClient, cfg, resp)
}

func parseConfig(args []string) (*config, error) {
	fs := flag.NewFlagSet("image-promotion", flag.ContinueOnError)
	cfg := &config{}

	fs.StringVar(&cfg.Sources, "sources", envOr("IMG_SOURCES", ""), "Comma-separated source image references")
	fs.StringVar(&cfg.Destinations, "destinations", envOr("IMG_DESTINATIONS", ""), "Comma-separated destination image:tag pairs")
	fs.StringVar(&cfg.Registries, "registries", envOr("IMG_REGISTRIES", ""), "Comma-separated target registry IDs (required)")
	fs.StringVar(&cfg.TagReference, "tag-reference", envOr("IMG_TAG_REFERENCE", ""), "Existing image:tag to retag")
	fs.StringVar(&cfg.NewTags, "new-tags", envOr("IMG_NEW_TAGS", ""), "Comma-separated new tags for retag")
	fs.StringVar(&cfg.Variables, "variables", envOr("IMG_VARIABLES", ""), "Comma-separated KEY=value pairs for %KEY% substitution")
	fs.StringVar(&cfg.MergeStrategy, "merge-strategy", envOr("IMG_MERGE_STRATEGY", ""), "Merge strategy: index_docker, index_oci, none")
	fs.StringVar(&cfg.Signing, "signing", envOr("IMG_SIGNING", "false"), "Enable image signing: true/false")
	fs.StringVar(&cfg.GatewayURL, "gateway-url", envOr("ARTIFACT_GATEWAY_URL", ""), "Gateway base URL (required)")
	fs.StringVar(&cfg.Token, "token", envOr("ARTIFACT_GATEWAY_TOKEN", ""), "Bearer token for gateway auth")
	fs.IntVar(&cfg.Timeout, "timeout", envOrInt("IMAGE_PROMOTION_TIMEOUT", 1800), "Polling timeout in seconds")
	fs.IntVar(&cfg.PollInterval, "poll-interval", envOrInt("IMAGE_PROMOTION_POLL_INTERVAL", 30), "Polling interval in seconds")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if cfg.GatewayURL == "" {
		return nil, fmt.Errorf("--gateway-url or ARTIFACT_GATEWAY_URL is required")
	}
	if cfg.Registries == "" {
		return nil, fmt.Errorf("--registries or IMG_REGISTRIES is required")
	}

	return cfg, nil
}

func buildRequest(cfg *config) (*request, error) {
	req := &request{
		Registries: splitNonEmpty(cfg.Registries),
	}

	if cfg.Sources != "" {
		req.Sources = splitNonEmpty(cfg.Sources)
		if cfg.Destinations != "" {
			req.Destinations = splitNonEmpty(cfg.Destinations)
		}
	} else if cfg.TagReference != "" {
		req.TagReference = cfg.TagReference
		if cfg.NewTags != "" {
			req.NewTags = splitNonEmpty(cfg.NewTags)
		}
	} else {
		return nil, fmt.Errorf("either --sources (publish) or --tag-reference (retag) is required")
	}

	if cfg.MergeStrategy != "" {
		req.MergeStrategy = cfg.MergeStrategy
	}
	if cfg.Signing == "true" {
		t := true
		req.Signing = &t
	}
	if cfg.Variables != "" {
		req.Variables = parseVariables(cfg.Variables)
	}

	return req, nil
}

func poll(httpClient *http.Client, cfg *config, resp *response) error {
	start := time.Now()
	timeoutDur := time.Duration(cfg.Timeout) * time.Second
	interval := time.Duration(cfg.PollInterval) * time.Second

	for time.Since(start) < timeoutDur {
		time.Sleep(interval)

		status, err := doGet(httpClient, cfg.GatewayURL+"/v1/image-publications/"+resp.ID, cfg.Token)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: poll failed: %v\n", err)
			continue
		}

		elapsed := int(time.Since(start).Seconds())
		fmt.Printf("[%ds/%ds] Publication %s: %s\n", elapsed, cfg.Timeout, resp.ID, status.Status)

		switch status.Status {
		case "success":
			fmt.Println("Image promotion completed successfully")
			return nil
		case "failed", "canceled":
			return fmt.Errorf("image promotion %s: %s", status.Status, resp.WebURL)
		}
	}

	return fmt.Errorf("timed out after %ds waiting for publication %s: %s", cfg.Timeout, resp.ID, resp.WebURL)
}

func doPost(client *http.Client, url, token string, body []byte) (*response, error) {
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", url, err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("gateway returned HTTP %d: %s", httpResp.StatusCode, string(respBody))
	}

	var resp response
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return &resp, nil
}

func doGet(client *http.Client, url, token string) (*response, error) {
	httpReq, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gateway returned HTTP %d: %s", httpResp.StatusCode, string(respBody))
	}

	var resp response
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return &resp, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var i int
		if _, err := fmt.Sscanf(v, "%d", &i); err == nil {
			return i
		}
	}
	return fallback
}

func splitNonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func parseVariables(s string) map[string]string {
	vars := make(map[string]string)
	for _, pair := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(pair, "=")
		if ok && k != "" {
			vars[k] = v
		}
	}
	return vars
}
