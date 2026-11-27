// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package config

const (
	// AgentUser is the Agent's account on macOS, which owns the configuration files the Agent
	// reads. macOS reserves the unprefixed namespace for the operating system, so it is _dd-agent.
	// The installer's .dmg set-up uses it too: the files Fleet writes must match what it secures.
	AgentUser = "_dd-agent"
	// AgentGroup is the group of the configuration files the Agent reads, the one the installer's
	// .dmg set-up (secureConfig) gives the whole configuration tree.
	AgentGroup = "admin"

	// agentConfigFileMode is the mode of every configuration file, the one the
	// installer's .dmg set-up (secureConfig) gives the whole configuration tree.
	agentConfigFileMode = 0660

	// configDirRights and configFileRights are the access the .dmg set-up (secureConfig) gives the
	// Agent's account on the configuration's directories and files, through an access control
	// entry.
	configDirRights  = "allow read,write,readattr,writeattr,readextattr,writeextattr,readsecurity,list,search,add_file,add_subdirectory,file_inherit,directory_inherit"
	configFileRights = "allow read,write,readattr,writeattr,readextattr,writeextattr,readsecurity"
	// inheritedFileRights is what a file inherits from configDirRights on its directory: list, add
	// file, search and add subdirectory become read, write, execute and append.
	inheritedFileRights = "allow read,write,execute,append,readattr,writeattr,readextattr,writeextattr,readsecurity"
	// AgentConfigDirACE and AgentConfigFileACE are those entries for the Agent's account.
	AgentConfigDirACE  = "user:" + AgentUser + " " + configDirRights
	AgentConfigFileACE = "user:" + AgentUser + " " + configFileRights
)
