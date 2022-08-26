// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// evalGoTemplate evaluates a go-style template on an object
func evalGoTemplate(s string, obj interface{}) string {
	tmpl, err := template.New("tmpl").Funcs(sprig.TxtFuncMap()).Parse(s)
	if err != nil {
		log.Warnf("failed to parse template %q: %v", s, err)
		return ""
	}

	b := &strings.Builder{}
	if err := tmpl.Execute(b, obj); err != nil {
		log.Tracef("failed to execute template %q: %v", s, err)
		return ""
	}
	return b.String()
}

// wrapErrorWithID wraps an error with an ID (e.g. rule ID)
func wrapErrorWithID(id string, err error) error {
	return fmt.Errorf("%s: %w", id, err)
}
