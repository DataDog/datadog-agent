# ------------------------------------
# OS-detection helper functions
# ------------------------------------
def linux?()
    return %w(rhel debian fedora suse gentoo slackware arch exherbo).include? ohai['platform_family']
end

def redhat?()
    return %w(rhel fedora).include? ohai['platform_family']
end

def suse?()
    return %w(suse).include? ohai['platform_family']
end

def debian?()
    return ohai['platform_family'] == 'debian'
end

def osx?()
    return ohai['platform_family'] == 'mac_os_x'
end

def windows?()
    return ohai['platform_family'] == 'windows'
end

def arm?()
    return ohai["kernel"]["machine"].start_with?("aarch", "arm")
end

def arm7l?()
    return ohai["kernel"]["machine"] == 'armv7l'
end

def os
    case RUBY_PLATFORM
    when /linux/
      'linux'
    when /darwin/
      'mac_os'
    when /x64-mingw32/
      'windows'
    else
      raise 'Unsupported OS'
    end
end

def with_python_runtime?(runtime)
    python_runtimes = ENV['PY_RUNTIMES'].nil? ? ['3'] : ENV['PY_RUNTIMES'].split(',')
    return python_runtimes.include? runtime
end
