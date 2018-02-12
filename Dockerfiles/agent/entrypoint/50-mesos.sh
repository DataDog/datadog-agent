#!/bin/bash

# Enable the Mesos integrations if relevant

cd /etc/datadog-agent/conf.d

if [[ $MESOS_MASTER ]]; then
  mv mesos_master.d/conf.yaml.example mesos_master.d/conf.yaml.default
  mv zk.d/conf.yaml.example zk.d/conf.yaml.default
  sed -i -e "s/localhost/leader.mesos/" mesos_master.d/conf.yaml.default
  sed -i -e "s/localhost/leader.mesos/" zk.d/conf.yaml.default
fi

if [[ $MESOS_SLAVE ]]; then
  mv mesos_slave.d/conf.yaml.example mesos_slave.d/conf.yaml.default
  sed -i -e "s/localhost/$HOST/" mesos_slave.d/conf.yaml.default
fi

if [[ $MARATHON_URL ]]; then
  mv marathon.d/conf.yaml.example marathon.d/conf.yaml.default
  sed -i -e "s@# - url: \"https://server:port\"@- url: ${MARATHON_URL}@" marathon.d/conf.yaml.default
fi
