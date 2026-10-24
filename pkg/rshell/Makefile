.PHONY: build fmt test test_all test_against_bash compliance

AGENT_ROOT := ../..

build:
	cd $(AGENT_ROOT) && bazel build //pkg/rshell/cmd/rshell

fmt:
	gofmt -w doc.go allowedpaths analysis builtins cmd internal interp privilegedhelper tests

test:
	cd $(AGENT_ROOT) && dda inv test --module=pkg/rshell --race

test_all:
	$(MAKE) test
	$(MAKE) test_against_bash

test_against_bash:
	cd $(AGENT_ROOT) && dda inv test --module=pkg/rshell --targets=tests --bazel-args="--test_env=RSHELL_BASH_TEST=1 --test_env=PATH --test_env=HOME --test_env=DOCKER_HOST --test_env=GOTMPDIR='$(CURDIR)' --strategy=TestRunner=standalone" --test-args="-test.run=TestShellScenariosAgainstBash -test.count=1"

compliance:
	cd $(AGENT_ROOT) && dda inv test --module=pkg/rshell --targets=tests --bazel-args="--test_env=RSHELL_COMPLIANCE_TEST=1" --test-args="-test.run=TestCompliance -test.count=1"
