#!/bin/sh
#set -e

##### Core config #####

if [ -z $DD_API_KEY ]; then
	echo "You must set DD_API_KEY environment variable to run the Datadog Agent container"
	exit 1
fi

if [ -z $DD_DD_URL ]; then
    export DD_DD_URL="https://app.datadoghq.com"
fi

if [ -z $DD_DOGSTATSD_SOCKET ]; then
    export DD_DOGSTATSD_NON_LOCAL_TRAFFIC=1
else
    if [ -e $DD_DOGSTATSD_SOCKET ]; then
        if [ -S $DD_DOGSTATSD_SOCKET ]; then
            echo "Deleting existing socket at ${DD_DOGSTATSD_SOCKET}"
            rm $DD_DOGSTATSD_SOCKET
        else
            echo "${DD_DOGSTATSD_SOCKET} exists and is not a socket, please check your volume options"
            exit 1
        fi
    fi
fi

##### Starting up dogstatsd #####

chmod +x /dogstatsd
sync	# Fix for 'Text file busy' error
exec "$@"
