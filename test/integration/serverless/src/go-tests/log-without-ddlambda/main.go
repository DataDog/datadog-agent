package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

type testResponse struct {
	StatusCode int    `json:"statusCode"`
	Body       string `json:"body"`
}

func testHandler(ctx context.Context, ev events.APIGatewayProxyRequest) (testResponse, error) {
	fmt.Printf("XXX LOG 0 XXX\n")
	fmt.Printf("XXX LOG 1 XXX\n")
	fmt.Printf("XXX LOG 2 XXX\n")
	return testResponse{
		StatusCode: 200,
		Body:       "ok",
	}, nil
}

func main() {
	lambda.Start(testHandler)
}
