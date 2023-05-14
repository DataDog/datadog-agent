name             "dd-system-probe-check"
maintainer       "Datadog"
description      "Executes system-probe (eBPF) integration tests"
long_description IO.read(File.join(File.dirname(__FILE__), 'README.md'))
version          "0.1.0"
depends "yum-centos", "~> 5.0.0"
depends 'docker'
