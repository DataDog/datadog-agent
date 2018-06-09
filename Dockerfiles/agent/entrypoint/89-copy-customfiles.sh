#!/bin/bash

#Expand environment variables
for file in /conf.d/*.yaml.tpl;
do
  conf_file=`echo $file | sed 's/\.tpl//'`
  envsubst < $file > $conf_file
done
# Copy the custom checks and confs in the /etc/datadog-agent folder
find /conf.d -name '*.yaml' -exec cp --parents -fv {} /etc/datadog-agent/ \;
find /checks.d -name '*.py' -exec cp --parents -fv {} /etc/datadog-agent/ \;
