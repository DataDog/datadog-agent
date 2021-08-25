#!/bin/bash

# Enable the Mesos integrations if relevant

CONFD=/etc/datadog-agent/conf.d

if [[ $MESOS_MASTER ]]; then
  if [[ ! -e $CONFD/mesos_master.d/conf.yaml.default ]]; then
    mv $CONFD/mesos_master.d/conf.yaml.example \
      $CONFD/mesos_master.d/conf.yaml.default
    sed -i -e "s/localhost/leader.mesos/" $CONFD/mesos_master.d/conf.yaml.default
  fi

  if [[ ! -e $CONFD/zk.d/conf.yaml.default ]]; then
    mv $CONFD/zk.d/conf.yaml.example \
      $CONFD/zk.d/conf.yaml.default
    sed -i -e "s/localhost/leader.mesos/" $CONFD/zk.d/conf.yaml.default
  fi
fi

if [[ $MESOS_SLAVE ]] && [[ ! -e $CONFD/mesos_slave.d/conf.yaml.default ]]; then
  mv $CONFD/mesos_slave.d/conf.yaml.example \
     $CONFD/mesos_slave.d/conf.yaml.default
  sed -i -e "s/localhost/$HOST/" $CONFD/mesos_slave.d/conf.yaml.default
fi

if [[ $MARATHON_URL ]] && [[ ! -e $CONFD/marathon.d/conf.yaml.default ]]; then
  mv $CONFD/marathon.d/conf.yaml.example \
     $CONFD/marathon.d/conf.yaml.default
  sed -i -E -e "s@ - url: (\")?https://<SERVER>:<PORT>(\")?@- url: ${MARATHON_URL}@" $CONFD/marathon.d/conf.yaml.default
fi
