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
    python_runtimes = ENV['PYTHON_RUNTIMES'].nil? ? ['2'] : ENV['PYTHON_RUNTIMES'].split(',')
    return python_runtimes.include? runtime
end

# ------------------------------------
# OS ops helper functions
# ------------------------------------
def read_output_lines(command)
  cmd = shellout(command)
  cmd.stdout.each_line do |line|
    yield line
  end
end
alias read_elf_files read_output_lines
