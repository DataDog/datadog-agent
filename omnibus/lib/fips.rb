require './lib/ostools.rb'

# Check that the build tags had an actual effect:
# the build tags added by fips mode (https://github.com/DataDog/datadog-agent/blob/7.75.1/tasks/build_tags.py#L140)
# only have the desired effect with the microsoft go compiler
# and may be silently ignored by other compilers.
# As a consequence the build succeeding isn't enough of a guarantee,
# we need to check the symbols for a proof that openSSL is used.
# (in practice, the default compiler fails compilation on Linux since at least 1.24 when it was made FIPS-aware)
def fips_check_binary_for_expected_symbol(path)
  if linux_target?
    symbol = "vendor/github.com/microsoft/go-crypto-openssl/openssl"
  elsif windows_target?
    # This is currently deadcode, see omnibus/config/projects/agent.rb
    symbol = "github.com/microsoft/go-crypto-winnative"
  else
    raise "Unsupported OS for FIPS"
  end

  inspector = Proc.new{ |symbols|
    count = symbols.scan(symbol).count
    if count > 0
      log.info(log_key) { "Symbol '#{symbol}' found #{count} times in binary '#{path}'." }
    else
      raise FIPSSymbolsNotFound.new("Expected to find '#{symbol}' symbol in '#{path}' but did not")
    end
  }
  GoSymbolsInspector.new(omnibazel_flags, fips_mode?, path, &inspector).inspect()
end
