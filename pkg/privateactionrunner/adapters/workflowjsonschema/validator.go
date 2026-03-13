// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package workflowjsonschema

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

func Validate(schema *jsonschema.Schema, data any) error {
	return FormatValidationError(schema.Validate(data))
}

func FormatValidationError(err error) error {
	if err == nil {
		return nil
	}
	var ve *jsonschema.ValidationError
	ok := errors.As(err, &ve)
	if !ok {
		return err
	}
	if ve.KeywordLocation == "/required" {
		return errors.New(ve.Message)
	}
	// /conditions/comparator/0/foo -> .conditions.comparator.0.foo
	loc := strings.ReplaceAll(ve.InstanceLocation, "/", ".")
	if strings.HasSuffix(ve.KeywordLocation, "/anyOf") {
		return fmt.Errorf("%s: did not match any specified AnyOf schemas", loc)
	}
	if strings.HasSuffix(ve.KeywordLocation, "/additionalProperties") {
		return errors.New(ve.Message)
	}
	if len(ve.Causes) == 0 {
		return fmt.Errorf("%s: %s", loc, ve.Message)
	}
	var errs *multierror.Error
	for _, c := range ve.Causes {
		if cErr := FormatValidationError(c); cErr != nil {
			errs = multierror.Append(errs, cErr)
		}
	}
	return errs.ErrorOrNil()
}
