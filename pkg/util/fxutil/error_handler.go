// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxutil

import (
	"fmt"
	"os"

	"go.uber.org/fx"
)

type errorHandler struct{}

func (errorHandler) HandleError(error) {
	if os.Getenv("TRACE_FX") == "" {
		fmt.Println("------------------------------------------------------")
		fmt.Printf("Set %v to have more information about FX errors.\n", traceFx)
		fmt.Println("------------------------------------------------------")
	}
}

// fxErrorHandler creates an fx.Option to display a message suggesting
// setting TRACE_FX to have more information about FX errors when
// TRACE_FX is not set.
func fxErrorHandler() fx.Option {
	return fx.ErrorHook(errorHandler{})
}
