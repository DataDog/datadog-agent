name             "dd-agent-import-conf"
maintainer       "Datadog"
description      "Import configuration from Agent 5 to Agent 6"
long_description IO.read(File.join(File.dirname(__FILE__), 'README.md'))
version          "0.1.0"

depends 'apt', '>= 2.1.0'
depends 'datadog'
depends 'yum', '< 7.0.0'
