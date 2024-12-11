// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"
	"fmt"
	"time"

	ddlambda "github.com/DataDog/datadog-lambda-go"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

type testResponse struct {
	StatusCode int    `json:"statusCode"`
	Body       string `json:"body"`
}

func testHandler(_ context.Context, _ events.APIGatewayProxyRequest) (testResponse, error) {
	// Sleep to ensure correct log ordering
	time.Sleep(250 * time.Millisecond)
	fmt.Printf("XXX LOG 0 XXX\n")
	time.Sleep(250 * time.Millisecond)
	fmt.Printf("XXX LOG 1 XXX\n")
	time.Sleep(250 * time.Millisecond)
	fmt.Printf("XXX LOG 2 XXX\n")
	time.Sleep(250 * time.Millisecond)
	return testResponse{
		StatusCode: 200,
		Body:       "ok",
	}, nil
}

func main() {
	lambda.Start(ddlambda.WrapHandler(testHandler, nil))
}
