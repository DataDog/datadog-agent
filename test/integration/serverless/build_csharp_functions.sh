#!/bin/bash

echo "Building C# Lambda functions"
cd src/csharp-tests
dotnet restore --verbosity quiet
set +e #set this so we don't exit if the tools are already installed
dotnet tool install -g Amazon.Lambda.Tools --framework netcoreapp3.1 --verbosity quiet
set -e
dotnet lambda package --configuration Release --framework netcoreapp3.1 --verbosity quiet --output-package bin/Release/netcoreapp3.1/handler.zip