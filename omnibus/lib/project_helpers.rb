#
# Project related helpers
#

require './lib/ostools.rb'

def sysprobe_enabled?()
  # This doesn't account for Windows special case which build system probe as part of the
  # agent build process
  !heroku_target? && linux_target? && !ENV.fetch('SYSTEM_PROBE_BIN', '').empty?
end

def windows_signing_enabled?()
  return ENV['SIGN_WINDOWS_DD_WCS']
end

# Determines whether we're under "repackaging" mode for the Agent, which means that we'll try to
# use an existing package and just build the datadog-agent definition without any dependencies
def do_repackage?
  return !ENV.fetch('OMNIBUS_REPACKAGE_SOURCE_URL', '').empty?
end
