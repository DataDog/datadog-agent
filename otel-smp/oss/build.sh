#!/bin/bash
docker login
docker build -t oliviergacadatadoghq/test .
docker push oliviergacadatadoghq/test