// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build three

package testdatadogagent

import "testing"

func TestSetExternalTagsUnicodeUnsuported(t *testing.T) {
	code := `
	tags = [
		('hostname1', {'source_type1': [u'tag1', 123, u'tag2\u00E1']}),
		('hostname2', {'source_type2': [u'tag3', [], u'tag4']}),
		('hostname3', {'source_type3': [1,2,3]}),
	]
	datadog_agent.set_external_tags(tags)
	`
	out, err := run(code)
	if err != nil {
		t.Fatal(err)
	}
	if out != "hostname1,source_type1,tag1,tag2\u00E1\nhostname2,source_type2,tag3,tag4\nhostname3,source_type3," {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
}
