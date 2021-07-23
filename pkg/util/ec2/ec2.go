// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type ec2Token struct {
	expirationDate time.Time
	value          string
	sync.RWMutex
}

// declare these as vars not const to ease testing
var (
	metadataURL        = "http://169.254.169.254/latest/meta-data"
	tokenURL           = "http://169.254.169.254/latest/api/token"
	oldDefaultPrefixes = []string{"ip-", "domu"}
	defaultPrefixes    = []string{"ip-", "domu", "ec2amaz-"}
	token              = ec2Token{}
	tokenRenewalWindow = 15 * time.Second
	// CloudProviderName contains the inventory name of for EC2
	CloudProviderName = "AWS"

	// cache keys
	instanceIDCacheKey = cache.BuildAgentKey("ec2", "GetInstanceID")
	hostnameCacheKey   = cache.BuildAgentKey("ec2", "GetHostname")
)

// GetInstanceID fetches the instance id for current host from the EC2 metadata API
func GetInstanceID(ctx context.Context) (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}

	instanceID, err := getMetadataItemWithMaxLength(ctx, "/instance-id", config.Datadog.GetInt("metadata_endpoints_max_hostname_size"))
	if err != nil {
		if instanceID, found := cache.Cache.Get(instanceIDCacheKey); found {
			log.Debugf("Unable to get ec2 instanceID from aws metadata, returning cached instanceID '%s': %s", instanceID, err)
			return instanceID.(string), nil
		}
		return "", err
	}

	cache.Cache.Set(instanceIDCacheKey, instanceID, cache.NoExpiration)

	return instanceID, nil
}

// GetLocalIPv4 gets the local IPv4 for the currently running host using the EC2 metadata API.
// Returns a []string to implement the HostIPProvider interface expected in pkg/process/util
func GetLocalIPv4() ([]string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return nil, fmt.Errorf("cloud provider is disabled by configuration")
	}
	ip, err := getMetadataItem(context.TODO(), "/local-ipv4")
	if err != nil {
		return nil, err
	}
	return []string{ip}, nil
}

// GetPublicIPv4 gets the public IPv4 for the currently running host using the EC2 metadata API.
func GetPublicIPv4(ctx context.Context) (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}
	return getMetadataItem(ctx, "/public-ipv4")
}

// IsRunningOn returns true if the agent is running on AWS
func IsRunningOn(ctx context.Context) bool {
	if _, err := GetHostname(ctx); err == nil {
		return true
	}
	return false
}

// GetHostname fetches the hostname for current host from the EC2 metadata API
func GetHostname(ctx context.Context) (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}

	hostname, err := getMetadataItemWithMaxLength(ctx, "/hostname", config.Datadog.GetInt("metadata_endpoints_max_hostname_size"))
	if err != nil {
		if hostname, found := cache.Cache.Get(hostnameCacheKey); found {
			log.Debugf("Unable to get ec2 hostname from aws metadata, returning cached hostname '%s': %s", hostname, err)
			return hostname.(string), nil
		}
		return "", err
	}

	cache.Cache.Set(hostnameCacheKey, hostname, cache.NoExpiration)

	return hostname, nil
}

// GetNetworkID retrieves the network ID using the EC2 metadata endpoint. For
// EC2 instances, the the network ID is the VPC ID, if the instance is found to
// be a part of exactly one VPC.
func GetNetworkID(ctx context.Context) (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}
	resp, err := getMetadataItem(ctx, "/network/interfaces/macs")
	if err != nil {
		return "", err
	}

	macs := strings.Split(strings.TrimSpace(resp), "\n")
	vpcIDs := common.NewStringSet()

	for _, mac := range macs {
		if mac == "" {
			continue
		}
		mac = strings.TrimSuffix(mac, "/")
		id, err := getMetadataItem(ctx, fmt.Sprintf("/network/interfaces/macs/%s/vpc-id", mac))
		if err != nil {
			return "", err
		}
		vpcIDs.Add(id)
	}

	switch len(vpcIDs) {
	case 0:
		return "", fmt.Errorf("EC2: GetNetworkID no mac addresses returned")
	case 1:
		return vpcIDs.GetAll()[0], nil
	default:
		return "", fmt.Errorf("EC2: GetNetworkID too many mac addresses returned")
	}
}

// Subnet stores information about an AWS subnet
type Subnet struct {
	ID   string
	Cidr string
}

// GetSubnetForHardwareAddr returns info about the subnet associated with a hardware
// address (mac address) on the current host
func GetSubnetForHardwareAddr(ctx context.Context, hwAddr net.HardwareAddr) (subnet Subnet, err error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		err = fmt.Errorf("cloud provider is disabled by configuration")
		return
	}

	if len(hwAddr) == 0 {
		err = fmt.Errorf("could not get subnet for empty hw addr")
		return
	}

	var resp string
	resp, err = getMetadataItem(ctx, fmt.Sprintf("/network/interfaces/macs/%s/subnet-id", hwAddr))
	if err != nil {
		return
	}

	subnet.ID = strings.TrimSpace(resp)

	resp, err = getMetadataItem(ctx, fmt.Sprintf("/network/interfaces/macs/%s/subnet-ipv4-cidr-block", hwAddr))
	if err != nil {
		return
	}

	subnet.Cidr = strings.TrimSpace(resp)
	return
}

// GetNTPHosts returns the NTP hosts for EC2 if it is detected as the cloud provider, otherwise an empty array.
// Docs: https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/set-time.html#configure_ntp
func GetNTPHosts(ctx context.Context) []string {
	if IsRunningOn(ctx) {
		return []string{"169.254.169.123"}
	}

	return nil
}

func getMetadataItemWithMaxLength(ctx context.Context, endpoint string, maxLength int) (string, error) {
	result, err := getMetadataItem(ctx, endpoint)
	if err != nil {
		return result, err
	}
	if len(result) > maxLength {
		return "", fmt.Errorf("%v gave a response with length > to %v", endpoint, maxLength)
	}
	return result, err
}

func getMetadataItem(ctx context.Context, endpoint string) (string, error) {
	res, err := doHTTPRequest(ctx, metadataURL+endpoint, http.MethodGet, map[string]string{}, config.Datadog.GetBool("ec2_prefer_imdsv2"))
	if err != nil {
		return "", fmt.Errorf("unable to fetch EC2 API, %s", err)
	}

	defer res.Body.Close()
	all, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read response body, %s", err)
	}

	return string(all), nil
}

// GetClusterName returns the name of the cluster containing the current EC2 instance
func GetClusterName(ctx context.Context) (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}
	tags, err := fetchTagsFromCache(ctx)
	if err != nil {
		return "", err
	}

	return extractClusterName(tags)
}

func extractClusterName(tags []string) (string, error) {
	var clusterName string
	for _, tag := range tags {
		if strings.HasPrefix(tag, "kubernetes.io/cluster/") { // tag key format: kubernetes.io/cluster/clustername"
			key := strings.Split(tag, ":")[0]
			clusterName = strings.Split(key, "/")[2] // rely on ec2 tag format to extract clustername
			break
		}
	}

	if clusterName == "" {
		return "", errors.New("unable to parse cluster name from EC2 tags")
	}

	return clusterName, nil
}

func doHTTPRequest(ctx context.Context, url string, method string, headers map[string]string, useToken bool) (*http.Response, error) {
	client := http.Client{
		Transport: httputils.CreateHTTPTransport(),
		Timeout:   time.Duration(config.Datadog.GetInt("ec2_metadata_timeout")) * time.Millisecond,
	}

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}

	if useToken {
		token, err := getToken(ctx)
		if err != nil {
			log.Warnf("ec2_prefer_imdsv2 is set to true in the configuration but the agent was unable to proceed: %s", err)
		} else {
			headers["X-aws-ec2-metadata-token"] = token
		}
	}

	for header, value := range headers {
		req.Header.Add(header, value)
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	} else if res.StatusCode != 200 {
		return nil, fmt.Errorf("status code %d trying to fetch %s", res.StatusCode, url)
	}
	return res, nil
}

func getToken(ctx context.Context) (string, error) {

	token.RLock()
	// The token renewal window is open, refreshing the token
	if time.Now().Before(token.expirationDate) {
		val := token.value
		token.RUnlock()
		return val, nil
	}
	token.RUnlock()
	token.Lock()
	defer token.Unlock()
	// Token has been refreshed by another caller
	if time.Now().Before(token.expirationDate) {
		return token.value, nil
	}

	client := http.Client{
		Transport: httputils.CreateHTTPTransport(),
		Timeout:   time.Duration(config.Datadog.GetInt("ec2_metadata_timeout")) * time.Millisecond,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, tokenURL, nil)
	if err != nil {
		return "", err
	}

	tokenLifetime := time.Duration(config.Datadog.GetInt("ec2_metadata_token_lifetime")) * time.Second
	req.Header.Add("X-aws-ec2-metadata-token-ttl-seconds", fmt.Sprintf("%d", int(tokenLifetime.Seconds())))
	// Set the local expiration date before requesting the metadata endpoint so the local expiration date will always
	// expire before the expiration date computed on the AWS side. The expiration date is set minus the renewal window
	// to ensure the token will be refreshed before it expires.
	token.expirationDate = time.Now().Add(tokenLifetime - tokenRenewalWindow)
	res, err := client.Do(req)
	if err != nil {
		// Re-mark the token as expired now, so it will be refreshed next time
		token.expirationDate = time.Now()
		return "", err
	}

	if res.StatusCode != 200 {
		// Re-mark the token as expired now, so it will be refreshed next time
		token.expirationDate = time.Now()
		return "", fmt.Errorf("status code %d trying to fetch %s", res.StatusCode, tokenURL)
	}

	defer res.Body.Close()
	all, err := ioutil.ReadAll(res.Body)
	if err != nil {
		// Re-mark the token as expired now, so it will be refreshed next time
		token.expirationDate = time.Now()
		return "", fmt.Errorf("unable to read response body, %s", err)
	}
	token.value = string(all)
	return token.value, nil
}

// IsDefaultHostname returns whether the given hostname is a default one for EC2
func IsDefaultHostname(hostname string) bool {
	return isDefaultHostname(hostname, config.Datadog.GetBool("ec2_use_windows_prefix_detection"))
}

// IsDefaultHostnameForIntake returns whether the given hostname is a default one for EC2 for the intake
func IsDefaultHostnameForIntake(hostname string) bool {
	return isDefaultHostname(hostname, false)
}

// IsWindowsDefaultHostname returns whether the given hostname is a Windows default one for EC2 (starts with 'ec2amaz-')
func IsWindowsDefaultHostname(hostname string) bool {
	return !isDefaultHostname(hostname, false) && isDefaultHostname(hostname, true)
}

func isDefaultHostname(hostname string, useWindowsPrefix bool) bool {
	hostname = strings.ToLower(hostname)
	isDefault := false

	var prefixes []string

	if useWindowsPrefix {
		prefixes = defaultPrefixes
	} else {
		prefixes = oldDefaultPrefixes
	}

	for _, val := range prefixes {
		isDefault = isDefault || strings.HasPrefix(hostname, val)
	}
	return isDefault
}

// HostnameProvider gets the hostname
func HostnameProvider(ctx context.Context, options map[string]interface{}) (string, error) {
	log.Debug("GetHostname trying EC2 metadata...")
	return GetInstanceID(ctx)
}
