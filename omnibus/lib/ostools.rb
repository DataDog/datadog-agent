# ------------------------------------
# OS-detection helper functions
# ------------------------------------
def linux_target?()
    return %w(rhel debian fedora suse gentoo slackware arch exherbo).include? ohai['platform_family']
end

def redhat_target?()
    return %w(rhel fedora).include? ohai['platform_family']
end

def suse_target?()
    return %w(suse).include? ohai['platform_family']
end

def debian_target?()
    return ohai['platform_family'] == 'debian'
end

def osx_target?()
    return ohai['platform_family'] == 'mac_os_x'
end

def windows_target?()
    return ohai['platform_family'] == 'windows'
end

def arm_target?()
    return ohai["kernel"]["machine"].start_with?("aarch", "arm")
end

def arm7l_target?()
    return ohai["kernel"]["machine"] == 'armv7l'
end

def heroku_target?()
    return ENV['AGENT_FLAVOR'] == 'heroku'
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
