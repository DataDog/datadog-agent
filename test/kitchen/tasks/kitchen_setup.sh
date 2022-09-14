#!/bin/bash -l

rm -rf "$CI_PROJECT_DIR/kitchen_logs"
rm -rf "$CI_PROJECT_DIR/test/kitchen/.kitchen"
mkdir "$CI_PROJECT_DIR/kitchen_logs"
ln -s "$CI_PROJECT_DIR/kitchen_logs" "$CI_PROJECT_DIR/test/kitchen/.kitchen"
