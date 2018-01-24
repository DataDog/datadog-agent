#!/bin/bash

# Enable the Mesos integrations if relevant

CONFD_PATH=/etc/datadog-agent/conf.d

if [[ $MESOS_MASTER ]]; then
  mv ${CONFD_PATH}/mesos_master.d/conf.yaml.example ${CONFD_PATH}/mesos_master.d/conf.yaml.default
  mv ${CONFD_PATH}/zk.d/conf.yaml.example ${CONFD_PATH}/zk.d/conf.yaml.default
  sed -i -e "s/localhost/leader.mesos/" ${CONFD_PATH}/mesos_master.d/conf.yaml.default
  sed -i -e "s/localhost/leader.mesos/" ${CONFD_PATH}/zk.d/conf.yaml.default
fi

if [[ $MESOS_SLAVE ]]; then
  mv ${CONFD_PATH}/mesos_slave.d/conf.yaml.example ${CONFD_PATH}/mesos_slave.d/conf.yaml.default
  sed -i -e "s/localhost/$HOST/" ${CONFD_PATH}/mesos_slave.d/conf.yaml.default
fi

if [[ $MARATHON_URL ]]; then
  mv ${CONFD_PATH}/marathon.d/conf.yaml.example ${CONFD_PATH}/marathon.d/conf.yaml.default
  sed -i -e "s@# - url: \"https://server:port\"@- url: ${MARATHON_URL}@" ${CONFD_PATH}/marathon.d/conf.yaml.default
fi
