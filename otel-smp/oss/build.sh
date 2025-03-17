#!/bin/bash
docker login
docker build --platform linux/amd64 -t oliviergacadatadoghq/test .
docker push oliviergacadatadoghq/test