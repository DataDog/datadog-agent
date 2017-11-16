// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.
// +build !windows

package app

type Servicedef struct {
	name			string
	configKey		string

}

var subservices = []Servicedef{
	Servicedef{
		name:		"apm",
		configKey:	"apm_enabled",
	},
	Servicedef{
		name:		"logs",
		configKey:	"log_agent_enabled",
	},
	Servicedef{
		name:		"process",
		configKey:	"process_agent_enabled",
	}}


func (s *Servicedef) Start() error {
	return nil
}

func (s *Servicedef) Stop() error {
	return nil
}