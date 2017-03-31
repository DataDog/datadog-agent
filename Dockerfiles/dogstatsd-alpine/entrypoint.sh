#!/bin/sh
#set -e


##### Core config #####

touch /datadog.yaml



if [[ $DD_API_KEY ]]; then
	echo "api_key: ${API_KEY}/" >> /datadog.yaml
else
	echo "You must set DD_API_KEY environment variable to run the Datadog Agent container"
	exit 1
fi

if [[ $DD_URL ]]; then
    echo "dd_url: ${DD_URL}" >> /datadog.yaml
else
    echo "dd_url: https://app.datadoghq.com" >> /datadog.yaml

fi


##### Starting up #####

exec "$@"
