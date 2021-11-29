#!/bin/bash

/lambda-entrypoint.sh /var/task/app.handler&
sleep 3
curl -XPOST "http://localhost:8080/2015-03-31/functions/function/invocations" -d '{}'