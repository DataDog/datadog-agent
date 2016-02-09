# ------------------------------------
# OS-detection helper functions 
# ------------------------------------
def linux?()
    return %w(rhel debian fedora suse gentoo slackware arch exherbo).include? ohai['platform_family']
end

def redhat?()
    return %w(rhel fedora suse).include? ohai['platform_family']
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
