#!/bin/bash

echo "Building Java Lambda Functions"
java_test_dirs=("metric" "trace" "log" "timeout" "error" "appsec")
cd src
for java_dir in "${java_test_dirs[@]}"; do
    mvn package -q -f java-tests/"${java_dir}"/pom.xml
done
