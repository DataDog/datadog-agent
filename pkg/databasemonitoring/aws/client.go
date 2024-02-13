package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
)

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=rdsclient_mockgen.go
type RDSClient interface {
	GetAuroraClusterEndpoints(dbClusterIdentifiers []string) (map[string]*AuroraCluster, error)
}

type Client struct {
	client *rds.RDS
}

// NewRDSClient creates a new AWS client for querying RDS
func NewRDSClient(region, roleArn string) (*Client, error) {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		Config: aws.Config{
			Region: aws.String(region),
		},
	}))
	creds := stscreds.NewCredentials(sess, roleArn)
	rdsSvc := rds.New(sess, &aws.Config{Credentials: creds})
	return &Client{client: rdsSvc}, nil
}
