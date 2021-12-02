#!/bin/sh

# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

##### Core config #####

if [ -z "$DD_API_KEY" ]; then
	echo "You must set DD_API_KEY environment variable to run the Datadog Agent container"
	exit 1
fi

if [ -z "$DD_DOGSTATSD_SOCKET" ]; then
    export DD_DOGSTATSD_NON_LOCAL_TRAFFIC=1
fi

##### Starting up dogstatsd #####

chmod +x /dogstatsd
sync	# Fix for 'Text file busy' error
exec "$@"
