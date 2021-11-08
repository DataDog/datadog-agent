#!/bin/bash

interfaces=(
	"AuditClient"
	"Builder"
	"Clients"
	"Configuration"
	"DockerClient"
	"Env"
	"Evaluatable"
	"Iterator"
	"KubeClient"
	"RegoConfiguration"
	"Reporter"
	"Scheduler"
)

name=^$(echo "${interfaces[@]}" | sed "s/ /$|^/g")$
echo $name

mockery --case snake -r --name="$name"
