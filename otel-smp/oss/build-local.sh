#!/bin/bash
docker login
docker build -t oliviergacadatadoghq/test-local .
docker push oliviergacadatadoghq/test-local