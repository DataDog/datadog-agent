#
# Profect related helpers
#

require './lib/ostools.rb'

def sysprobe_enabled?()
  # This doesn't account for Windows special case which build system probe as part of the
  # agent build process
  !heroku_target? && linux_target? && ENV.fetch('SYSTEM_PROBE_BIN', '').empty?
end

