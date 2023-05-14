name             "dd-agent-install-script"
maintainer       "Datadog"
description      "Installs the Agent using the Agent install script"
long_description IO.read(File.join(File.dirname(__FILE__), 'README.md'))
version          "0.2.0"
depends          "yum-centos", "~> 5.0.0"
depends          "docker"

