package aws

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/rds"
	"regexp"

	"strings"
)

const regexPattern = `^([a-z]+-[a-z]+-\d+)[a-z]$`

var awsRegionRegex = regexp.MustCompile(regexPattern)

// AuroraCluster represents an Aurora cluster
type AuroraCluster struct {
	Instances []Instance `json:"instances,omitempty"`
}

// Instance represents an Aurora instance
type Instance struct {
	Endpoint   string `json:"endpoint,omitempty"`
	Port       int64  `json:"port,omitempty"`
	Region     string `json:"region,omitempty"`
	IamEnabled bool   `json:"iam_enabled,omitempty"`
}

// GetAuroraClusterEndpoints queries an AWS account for the endpoints of an Aurora cluster
// requires the dbClusterIdentifier for the cluster
func (c *Client) GetAuroraClusterEndpoints(dbClusterIdentifiers []string) (map[string]*AuroraCluster, error) {
	if len(dbClusterIdentifiers) == 0 {
		return nil, fmt.Errorf("at least one database clusterInstances identifier is required")
	}
	idVals := make([]*string, len(dbClusterIdentifiers))
	for i, id := range dbClusterIdentifiers {
		idVals[i] = aws.String(id)
	}
	clusterInstances, err := c.client.DescribeDBInstances(
		&rds.DescribeDBInstancesInput{
			Filters: []*rds.Filter{
				{
					Name:   aws.String("db-cluster-id"),
					Values: idVals,
				},
			},
		})
	if err != nil {
		return nil, fmt.Errorf("error describing aurora DB clusters: %v", err)
	}
	clusters := make(map[string]*AuroraCluster, 0)
	for _, db := range clusterInstances.DBInstances {
		if db.Endpoint != nil && db.DBClusterIdentifier != nil {
			if db.Endpoint.Address == nil {
				continue
			}
			// Add to list of instances for the cluster
			instance := Instance{
				Endpoint: *db.Endpoint.Address,
			}
			if _, ok := clusters[*db.DBClusterIdentifier]; !ok {
				clusters[*db.DBClusterIdentifier] = &AuroraCluster{
					Instances: make([]Instance, 0),
				}
			}
			// Set if IAM is configured for the endpoint
			clusters[*db.DBClusterIdentifier].Instances = append(clusters[*db.DBClusterIdentifier].Instances, instance)
			if db.IAMDatabaseAuthenticationEnabled != nil {
				instance.IamEnabled = *db.IAMDatabaseAuthenticationEnabled
			}
			// Set the port, if it is known
			if db.Endpoint.Port != nil {
				instance.Port = *db.Endpoint.Port
			}
			// Set the region, if it is known
			if db.AvailabilityZone != nil {
				region, err := parseAWSRegion(*db.AvailabilityZone)
				if err != nil {
					_ = log.Errorf("Error parsing AWS region from availability zone: %s", *db.AvailabilityZone)
					continue
				}
				instance.Region = region
			}
		}
	}

	if len(clusters) == 0 {
		log.Debugf("No endpoints found for the Aurora clusters with ids %s", strings.Join(dbClusterIdentifiers, ", "))
	}

	return clusters, nil
}

func parseAWSRegion(availabilityZone string) (string, error) {
	// Use the awsRegionRegex pattern to find matches in the availability zone.
	matches := awsRegionRegex.FindStringSubmatch(availabilityZone)
	if len(matches) == 2 {
		return matches[1], nil
	}
	return "", fmt.Errorf("unable to parse AWS region from availability zone: %s", availabilityZone)
}
