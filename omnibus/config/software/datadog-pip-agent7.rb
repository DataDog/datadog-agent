# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

name 'datadog-pip-agent7'

dependency 'datadog-pip'
dependency 'datadog-agent'

version="0.0.3"

build do
  pip "install datadog-a7==#{version}"
end
