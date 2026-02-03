require './lib/ostools.rb'

# Use msgo toolchain when fips mode is enabled
def fips_add_msgo_to_env(env)
  if !fips_mode?
    return
  end

  if linux_target?
    msgo_root = '/usr/local/msgo'
    binary_name = 'go'
    delim = ':'
  elsif windows_target?
    msgo_root = ENV['MSGO_ROOT']
    binary_name = 'go.exe'
    delim = ';'

    if msgo_root.nil? || msgo_root.empty?
      raise "MSGO_ROOT not set"
    end
  else
    raise "Unsupported OS for FIPS"
  end

  msgo_path = Pathname.new(msgo_root) + 'bin'
  msgo_bin_path = msgo_path + binary_name
  if !File.exist?(msgo_bin_path)
    raise "msgo #{binary_name} not found in #{msgo_path}"
  end

  env['GOROOT'] = msgo_root
  env['PATH'] = "#{msgo_path.realpath()}#{delim}#{env['PATH']}"
  # also update the global env so that the symbol inspector use the correct go version
  ENV['GOROOT'] = msgo_root
  ENV['PATH'] = "#{msgo_path.realpath()}#{delim}#{ENV['PATH']}"
end

# Check that the build tags had an actual effect:
# the build tags added by fips mode (https://github.com/DataDog/datadog-agent/blob/7.75.1/tasks/build_tags.py#L140)
# only have the desired effect with the microsoft go compiler
# and may be silently ignored by other compilers.
# As a consequence the build succeeding isn't enough of a guarantee,
# we need to check the symbols for a proof that openSSL is used.
# (in practice, the default compiler fails compilation on Linux since at least 1.24 when it was made FIPS-aware)
def fips_check_binary_for_expected_symbol(path)
  if linux_target?
    symbol = "_Cfunc__mkcgo_OPENSSL" # since Go 1.25
  elsif windows_target?
    symbol = "github.com/microsoft/go-crypto-winnative"
  else
    raise "Unsupported OS for FIPS"
  end

  GoSymbolsInspector.new(path).inspect do |symbols|
      count = symbols.scan(symbol).count
      if count > 0
        log.info(log_key) { "Symbol '#{symbol}' found #{count} times in binary '#{path}'." }
      else
        raise FIPSSymbolsNotFound.new("Expected to find '#{symbol}' symbol in #{path} but did not")
      end
    end
end
