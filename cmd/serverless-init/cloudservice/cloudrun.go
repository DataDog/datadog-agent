package cloudservice

import "github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice/helper"

// CloudRun has helper functions for getting Google Cloud Run data
type CloudRun struct{}

// GetTags returns a map of gcp-related tags.
func (c *CloudRun) GetTags() map[string]string {
	return helper.GetMetaData(helper.GetDefaultConfig()).TagMap()
}

// GetOrigin returns the `origin` attribute type for the given
// cloud service.
func (c *CloudRun) GetOrigin() string {
	return "cloudrun"
}
