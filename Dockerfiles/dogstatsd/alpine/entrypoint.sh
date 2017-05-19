#!/bin/sh
#set -e

##### Core config #####

if [[ -z $DD_API_KEY ]]; then
	echo "You must set DD_API_KEY environment variable to run the Datadog Agent container"
	exit 1
fi

if [[ -z $DD_DD_URL ]]; then
    export DD_DD_URL="https://app.datadoghq.com"
fi

if [ -z $DD_DOGSTATSD_SOCKET ]; then
    export DD_DOGSTATSD_NON_LOCAL_TRAFFIC=1
fi

##### Starting up dogstatsd #####

chmod +x /dogstatsd
sync	# Fix for 'Text file busy' error
exec "$@"
