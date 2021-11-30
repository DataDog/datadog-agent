#!/bin/bash -e

# since Go 1.13, the -exec flag of go test could add some parameters such as -test.timeout
# to the call, we don't want them because while calling invoke below, invoke
# thinks that the parameters are for it to interpret.
# we're calling an intermediate script which only pass the binary name to the invoke task.

invoke -e docker.dockerize-test "$1"
