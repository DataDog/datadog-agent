# StackState Agent

[![CircleCI](https://circleci.com/gh/StackVista/stackstate-agent/tree/master.svg?style=svg)](https://circleci.com/gh/StackVista/stackstate-agent/tree/master)
[![Build status](https://ci.appveyor.com/api/projects/status/kcwhmlsc0oq3m49p/branch/master?svg=true)](https://ci.appveyor.com/project/StackVista/stackstate-agent/branch/master)
[![GoDoc](https://godoc.org/github.com/StackVista/stackstate-agent?status.svg)](https://godoc.org/github.com/StackVista/stackstate-agent)
[![Go Report Card](https://goreportcard.com/badge/github.com/StackVista/stackstate-agent)](https://goreportcard.com/report/github.com/StackVista/stackstate-agent)

The present repository contains the source code of the StackState Agent version 6.

## Documentation

The general documentation of the project, including instructions for installation
and development, is located under [the docs directory](docs) of the present repo.

## Getting started

To build the Agent you need:
 * [Go](https://golang.org/doc/install) 1.10.2 or later.
 * Python 2.7 along with development libraries.
 * Python dependencies. You may install these with `pip install -r requirements.txt`
   This will also pull in [Invoke](http://www.pyinvoke.org) if not yet installed.

**Note:** you may want to use a python virtual environment to avoid polluting your
      system-wide python environment with the agent build/dev dependencies.

**Note:** You may have previously installed `invoke` via brew on MacOS, or `pip` in
      any other platform. We recommend you use the version pinned in the requirements
      file for a smooth development/build experience. 

Builds and tests are orchestrated with `invoke`, type `invoke --list` on a shell
to see the available tasks.

To start working on the Agent, you can build the `master` branch:

1. checkout the repo: `git clone https://github.com/StackVista/stackstate-agent.git $GOPATH/src/github.com/StackVista/stackstate-agent`.
2. cd into the project folder: `cd $GOPATH/src/github.com/StackVista/stackstate-agent`.
3. install project's dependencies: `invoke deps`.
   Make sure that `$GOPATH/bin` is in your `$PATH` otherwise this step might fail.
4. build the whole project with `invoke agent.build --build-exclude=snmp,systemd`

Please refer to the [Agent Developer Guide](docs/dev/README.md) for more details.

## Run

To start the agent type `agent run` from the `bin/agent` folder, it will take
care of adjusting paths and run the binary in foreground.

You need to provide a valid API key. You can either use the config file or
overwrite it with the environment variable like:
```
STS_API_KEY=12345678990 ./bin/agent/agent -c bin/agent/dist/stackstate.yaml
```

## Contributing code

You'll find information and help on how to contribute code to this project under
[the `docs/dev` directory](docs/dev) of the present repo.

## Install

Prerequisites:

Before installing on debian distributions like `jessie` and `stretch` you have to:
    
    $ sudo apt-get install apt-transport-https
    
To install the debian package:

    $ wget -qO - https://s3.amazonaws.com/stackstate-agent-2/public.key | sudo apt-key add -
    $ echo "deb https://s3.amazonaws.com/stackstate-agent-2 stable main" | sudo tee -a /etc/apt/sources.list.d/stackstate-agent.list
    $ sudo apt-get update && sudo apt-get install stackstate-agent
    $ sudo cp /etc/stackstate-agent/stackstate.yaml.example /etc/stackstate-agent/stackstate.yaml
    $ sudo chown stackstate-agent:stackstate-agent /etc/stackstate-agent/stackstate.yaml
    $ sudo service stackstate-agent start
    
To install a PR branch version use another repository:

`https://s3.amazonaws.com/stackstate-agent-2-test PR_NAME main` 

and replace `PR_NAME` with the branch name (e.g. master, STAC-xxxx). 
