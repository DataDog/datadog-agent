package client

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

type ECSClient struct {
	ecs.Client
	clusterName string
}

func NewECSClient(clusterName string) (*ECSClient, error) {
	ctx := context.Background()
	cfg, err := awsConfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	return &ECSClient{Client: *ecs.NewFromConfig(cfg), clusterName: clusterName}, err
}

func (c *ECSClient) ExecCommand(task, container string, cmd string) (stdout, stderr string, err error) {
	output, err := c.ExecuteCommand(context.Background(), &ecs.ExecuteCommandInput{
		Cluster:   aws.String(c.clusterName),
		Container: aws.String(container),
		Task:      aws.String(task),
		Command:   aws.String(cmd),
	})
	if err != nil {
		return "", "", err
	}
	json.MarshalIndent()

}
