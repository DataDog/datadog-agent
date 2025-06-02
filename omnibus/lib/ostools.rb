# ------------------------------------
# OS-detection helper functions
# ------------------------------------
def linux_target?()
    return %w(rhel debian fedora suse gentoo slackware arch exherbo).include? ohai['platform_family']
end

def redhat_target?()
    if not Omnibus::Config.host_distribution().nil?
      return %w(rhel fedora ociru).include? Omnibus::Config.host_distribution()
    end
    return %w(rhel fedora).include? ohai['platform_family']
end

def suse_target?()
    if not Omnibus::Config.host_distribution().nil?
      return Omnibus::Config.host_distribution() == 'suse'
    end
    return %w(suse).include? ohai['platform_family']
end

def debian_target?()
    if not Omnibus::Config.host_distribution().nil?
      return Omnibus::Config.host_distribution() == 'debian'
    end
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

def ot_target?()
    return ENV['AGENT_FLAVOR'] == 'ot'
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

def fips_mode?()
  return ENV['AGENT_FLAVOR'] == "fips" && (linux_target? || windows_target?)
end