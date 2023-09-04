def build_with_autotools(**options)
  ##
  # Wrapper around autoconf/autoreconf/automake based projects
  #
  # These settings may be passed as the options hash:
  # :discard_default_opts
  # :configure_opts
  # :prefix
  # :CFLAGS
  # :LDFLAGS
  # :PKG_CONFIG
  # :PKG_CONFIG_PATH

  env = with_standard_compiler_flags()
  prefix = options.delete(:prefix) || "#{install_dir}/embedded"

  env_vars = {
    # A list of environment variables that can be extended/replaced
    # The key is the variable name, the boolean is true if the value
    # needs to be extended, false if it should be replaced
    'CFLAGS' => true,
    'CXXFLAGS' => true,
    'LDFLAGS' => true,
    'PKG_CONFIG_PATH' => true,
    'PKG_CONFIG' => false,
    'CC' => false,
    'CXX' => false,
    'CPP' => false
  }
  env_vars.each do |e, append|
    sym = e.to_sym
    unless options.key?(sym)
      next
    end
    unless append
      env[e] = options.delete(sym)
      next
    end
    unless env.key?(e)
      env[e] = ''
    end
    env[e] += " #{options.delete(sym)}"
  end

  #FIXME: should we add enable-shared/disable-static only for macOS & linux?
  configure_options = []
  unless options.key?(:discard_default_opts) and options[:discard_default_opts]
    configure_options = [
      "--disable-static",
      "--enable-shared",
      "--disable-dependency-tracking",
    ]
  end
  configure_options.append("--prefix=#{prefix}")
  configure_options += (options.delete(:configure_opts) || [])

  command "./configure ".concat(configure_options.join(' ').strip),
          :env => env
  make "-j #{workers}", :env => env
  unless options.key?(:skip_install) and options[:skip_install]
    make 'install', :env => env, :in_msys_bash => true
  end
end
