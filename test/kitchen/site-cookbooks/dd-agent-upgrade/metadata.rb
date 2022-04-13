name             "dd-agent-upgrade"
maintainer       "Datadog"
description      "Updates the installed Agent"
long_description IO.read(File.join(File.dirname(__FILE__), 'README.md'))
version          "0.2.0"

depends 'apt', '>= 2.1.0'
depends 'datadog'
depends 'yum', '< 7.0.0'
