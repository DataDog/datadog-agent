// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"fmt"
	"sort"
)

var configGroups Groups

func registerGroup(g *Group) {
	prepareOptions(g.Options, "")
	applyOptions(g.Options)
	configGroups = append(configGroups, g)
}

func GetGroups() []*Group {
	sort.Sort(configGroups)
	return configGroups
}

// prepareOptions populates the options' viperName fields to
// handle nested options transparently
func prepareOptions(options []*Option, prefix string) {
	for _, o := range options {
		o.viperName = o.Name
		if len(prefix) > 0 {
			o.viperName = fmt.Sprintf("%s.%s", prefix, o.Name)
		}
		prepareOptions(o.SubOptions, o.viperName)
	}
}

// applyOptions configures viper itself
func applyOptions(options []*Option) {
	for _, o := range options {
		if o.DefaultValue != nil {
			Datadog.SetDefault(o.viperName, o.DefaultValue)
		}
		if !o.NoEnvvar {
			Datadog.BindEnv(o.viperName)
		}
		applyOptions(o.SubOptions)
	}
}
