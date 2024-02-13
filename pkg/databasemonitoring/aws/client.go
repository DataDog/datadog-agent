package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
)

type Client struct {
	client *rds.RDS
}

// NewRDSClient creates a new AWS client for querying RDS
func NewRDSClient(region string) (*Client, error) {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		Config: aws.Config{
			Region: aws.String(region),
		},
	}))

	// Create a new RDS service client
	rdsSvc := rds.New(sess)
	return &Client{client: rdsSvc}, nil
}
