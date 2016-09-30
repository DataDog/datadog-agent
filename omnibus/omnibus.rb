#
# This file is used to configure the datadog-agent project. It contains
# some minimal configuration examples for working with Omnibus. For a full list
# of configurable options, please see the documentation for +omnibus/config.rb+.
#

# Windows architecture defaults
# ------------------------------
windows_arch   %w{x86 x64}.include?((ENV['OMNIBUS_WINDOWS_ARCH'] || '').downcase) ?
                 ENV['OMNIBUS_WINDOWS_ARCH'].downcase.to_sym : :x86
