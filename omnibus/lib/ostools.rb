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

# Machine name of the architecture we are building *for*, in ohai's spelling
# (e.g. "arm64", "x86_64", "armv7l"). Defaults to the host, so a native build is
# unaffected; OMNIBUS_TARGET_ARCH overrides it when cross-compiling (macOS
# x86_64 is built on Apple Silicon runners since the Intel fleet is retired).
def target_machine()
    return ENV['OMNIBUS_TARGET_ARCH'] || ohai["kernel"]["machine"]
end

def cross_compiling?()
    return target_machine != ohai["kernel"]["machine"]
end

def arm_target?()
    return target_machine.start_with?("aarch", "arm")
end

def arm7l_target?()
    return target_machine == 'armv7l'
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

def fips_mode?()
  return ENV['AGENT_FLAVOR'] == "fips" && (linux_target? || windows_target?)
end

# Expose --//packages/agent:flavor, --//:install_dir and --//:output_config_dir, pinned from the corresponding omnibus
# build environment variables.
# 💡 Mirrors `_insert_omnibazel_flags` in tasks/libs/build/bazel.py.
def omnibazel_flags()
  flags = []
  flags << "--//packages/agent:flavor=#{ENV['AGENT_FLAVOR']}" if ENV['AGENT_FLAVOR']
  # In macos, omnibus install_dir is the build location, which is different from the expected install location
  flags << (osx_target? ? "--//:install_dir=/opt/datadog-agent" : "--//:install_dir=#{install_dir}")
  flags << "--//:output_config_dir=#{ENV['OUTPUT_CONFIG_DIR']}"
  # Only pinned when cross-compiling: a native build keeps Bazel's host-derived
  # default, and so keeps its analysis cache.
  flags << "--platforms=//bazel/platforms:#{osx_target? ? 'macos' : 'linux'}_#{arm_target? ? 'arm64' : 'x86_64'}" if cross_compiling?
  flags.join(' ')
end
