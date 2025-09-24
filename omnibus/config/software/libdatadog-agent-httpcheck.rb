name "libdatadog-agent-httpcheck"
description "Pre-compiled shared library check"

# Version of your library (for tracking and caching purposes)
default_version "1.0.0"

# Source path - points to the directory containing your pre-compiled library
# This assumes you have a directory structure like:
# omnibus/files/#{name}/
# ├── #{name}.so      (Linux)
# ├── #{name}.dylib   (macOS)
# ├── #{name}.dll     (Windows)
source path: "#{project.files_path}/#{name}"

# Always build since we're just copying pre-compiled files
always_build true

build do
  # Set up environment with standard compiler flags and embedded path
  env = with_standard_compiler_flags(with_embedded_path)
  
  # Create necessary directories in the installation target
  mkdir "#{install_dir}/embedded/lib"
  
  # Platform-specific file copying
  if linux?
    # Copy Linux shared library (.so files)
    # Handle versioned libraries if they exist
    if File.exist?("#{project_dir}/#{name}.so")
      copy "#{name}.so*", "#{install_dir}/embedded/lib/"
    else
      raise "Linux shared library #{name}.so not found in #{project_dir}"
    end
    
    # Set proper permissions
    command "chmod 755 #{install_dir}/embedded/lib/#{name}.so*"
    end
  end

  if mac?
    # Copy macOS dynamic library (.dylib files)
    if File.exist?("#{project_dir}/#{name}.dylib")
      copy "#{name}.dylib", "#{install_dir}/embedded/lib/"
    else
      raise "macOS dynamic library #{name}.dylib not found in #{project_dir}"
    end
    
    # Set proper permissions
    command "chmod 755 #{install_dir}/embedded/lib/#{name}.dylib"
    
    # Update install name for the embedded path (important for macOS)
    command "install_name_tool -id #{install_dir}/embedded/lib/#{name}.dylib #{install_dir}/embedded/lib/#{name}.dylib"
  end

  if windows?
    # Copy Windows DLL and import library
    if File.exist?("#{project_dir}/#{name}.dll")
      copy "#{name}.dll", "#{install_dir}/embedded/lib/"
    else
      raise "Windows DLL #{name}.dll not found in #{project_dir}"
    end

  # Verify the library was copied correctly
  block do
    library_extensions = {
      'linux' => 'so',
      'mac' => 'dylib', 
      'windows' => 'dll'
    }
    
    current_platform = case node['platform']
                      when 'ubuntu', 'debian', 'centos', 'rhel', 'fedora', 'suse', 'opensuse'
                        'linux'
                      when 'mac_os_x'
                        'mac'
                      when 'windows'
                        'windows'
                      else
                        'unknown'
                      end
    
    if current_platform != 'unknown'
      ext = library_extensions[current_platform]
      lib_path = "#{install_dir}/embedded/lib/#{name}.#{ext}"
      
      unless File.exist?(lib_path)
        raise "Failed to install library: #{lib_path} not found after build"
      end
      
      puts "Successfully installed #{name} at: #{lib_path}"
    end
  end
end
