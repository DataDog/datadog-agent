package ec2

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const (
	imdsBaseURL = "http://169.254.169.254/latest/meta-data/"
)

var (
	getInstanceID = GetInstanceID
	getRegion     = GetRegion
	getAccountID  = GetAccountID
)

type cloudProviderCCRIDDetector func(context.Context) (string, error)

type Fetcher struct {
	Name    string
	Attempt func(context.Context) (interface{}, error)
	mu      sync.Mutex
	cache   interface{}
	err     error
	ready   bool
}

func (f *Fetcher) Fetch(ctx context.Context) (interface{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ready {
		return f.cache, f.err
	}
	f.cache, f.err = f.Attempt(ctx)
	f.ready = true
	return f.cache, f.err
}

func (f *Fetcher) FetchString(ctx context.Context) (string, error) {
	val, err := f.Fetch(ctx)
	if err != nil {
		return "", err
	}
	s, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("%s returned non-string", f.Name)
	}
	return s, nil
}

var regionFetcher = Fetcher{
	Name: "EC2 Region",
	Attempt: func(ctx context.Context) (interface{}, error) {
		return httpGetMetadata("placement/region")
	},
}

var accountIDFetcher = Fetcher{
	Name: "AWS Account ID",
	Attempt: func(ctx context.Context) (interface{}, error) {
		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			return "", err
		}
		client := sts.NewFromConfig(cfg)
		output, err := client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
		if err != nil {
			return "", err
		}
		return aws.ToString(output.Account), nil
	},
}

func GetRegion(ctx context.Context) (string, error) {
	return regionFetcher.FetchString(ctx)
}

func httpGetMetadata(path string) (string, error) {
	req, _ := http.NewRequest("GET", imdsBaseURL+path, nil)
	req.Header.Set("Metadata-Flavor", "Amazon")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("metadata %q request failed: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata %q returned status %s", path, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

// GetHostCCRID returns the EC2 instance ARN for use as host CCRID
func GetHostCCRID(ctx context.Context) (string, error) {
	instanceID, err := getInstanceID(ctx)
	if err != nil {
		return "", err
	}
	region, err := getRegion(ctx)
	if err != nil {
		return "", err
	}
	accountID, err := getAccountID(ctx)
	if err != nil {
		return "", err
	}

	arn := fmt.Sprintf("arn:aws:ec2:%s:%s:instance/%s", region, accountID, instanceID)
	return arn, nil
}
