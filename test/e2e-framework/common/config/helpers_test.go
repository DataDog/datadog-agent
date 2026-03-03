// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
)

func Test_tagListToKeyValueMap(t *testing.T) {
	t.Run("should parse valid tags", func(t *testing.T) {
		tagList := []string{"name:totoro", "country:jp"}
		tags, err := tagListToKeyValueMap(tagList)
		assert.NoError(t, err)
		assert.Equal(t, map[string]string{"name": "totoro", "country": "jp"}, tags)
	})

	t.Run("should return error when there is an invalid tag", func(t *testing.T) {
		tagList := []string{"name:totoro", "invalid_tag"}
		tags, err := tagListToKeyValueMap(tagList)
		assert.Error(t, err)
		assert.Equal(t, map[string]string{"name": "totoro"}, tags)
	})
}

func Test_extendTagsMap(t *testing.T) {
	t.Run("should add items to an empty map", func(t *testing.T) {
		pulumiMap := pulumi.StringMap{}
		extendTagsMap(pulumiMap, map[string]string{"name": "totoro", "country": "jp"})
		assert.Equal(t, pulumi.StringMap{"name": pulumi.String("totoro"), "country": pulumi.String("jp")}, pulumiMap)
	})

	t.Run("should add extra items to an existing map", func(t *testing.T) {
		pulumiMap := pulumi.StringMap{"name": pulumi.String("totoro"), "country": pulumi.String("jp")}
		extendTagsMap(pulumiMap, map[string]string{"team": "the_best"})
		assert.Equal(t, pulumi.StringMap{"name": pulumi.String("totoro"), "country": pulumi.String("jp"), "team": pulumi.String("the_best")}, pulumiMap)
	})

	t.Run("should overwrite values of existing keys", func(t *testing.T) {
		pulumiMap := pulumi.StringMap{"name": pulumi.String("totoro"), "origin_country": pulumi.String("jp")}
		extendTagsMap(pulumiMap, map[string]string{"name": "kiki"})
		assert.Equal(t, pulumi.StringMap{"name": pulumi.String("kiki"), "origin_country": pulumi.String("jp")}, pulumiMap)
	})

	t.Run("should lower keys and replace `_` with `-` in keys", func(t *testing.T) {
		pulumiMap := pulumi.StringMap{}
		extendTagsMap(pulumiMap, map[string]string{"NAME": "totoro", "origin_COUntry": "jp"})
		assert.Equal(t, pulumi.StringMap{"name": pulumi.String("totoro"), "origin-country": pulumi.String("jp")}, pulumiMap)
	})
}
