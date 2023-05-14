#!/bin/bash

rm -rf "$CI_PROJECT_DIR/kitchen_logs"
rm -rf "$DD_AGENT_TESTING_DIR/.kitchen"
mkdir "$CI_PROJECT_DIR/kitchen_logs"
ln -s "$CI_PROJECT_DIR/kitchen_logs" "$DD_AGENT_TESTING_DIR/.kitchen"
