name             "dd-security-agent-check"
maintainer       "Datadog"
description      "Test the runtime security agent"
long_description IO.read(File.join(File.dirname(__FILE__), 'README.md'))
version          "0.1.0"

depends 'datadog'
depends 'docker'
depends 'selinux'
