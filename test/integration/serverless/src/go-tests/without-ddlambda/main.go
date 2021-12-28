// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

type testResponse struct {
	StatusCode int    `json:"statusCode"`
	Body       string `json:"body"`
}

// PingHandler returns retuns a mock 200 response
//revive:disable
func PingHandler(ctx context.Context, ev events.APIGatewayProxyRequest) (testResponse, error) {
	return testResponse{
		StatusCode: 200,
		Body:       "ok",
	}, nil
}

//revive:enable

func main() {
	lambda.Start(PingHandler)
}
