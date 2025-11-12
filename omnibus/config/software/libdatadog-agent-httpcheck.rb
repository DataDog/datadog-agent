name "libdatadog-agent-httpcheck"
description "Pre-compiled shared library check"

# Version of your library (for tracking and caching purposes)
default_version "1.0.0"

# Source path - points to the directory containing your pre-compiled library
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
    if File.exist?("#{project.files_path}/#{name}/#{name}.so")
      copy "#{name}.so", "#{install_dir}/embedded/lib/"
      # Set proper permissions
      command "chmod 755 #{install_dir}/embedded/lib/#{name}.so"
    else
      raise "Linux shared library #{name}.so not found in #{project.files_path}/#{name}"
    end
  elsif mac?
    # Copy macOS dynamic library (.dylib files)
    if File.exist?("#{project.files_path}/#{name}/#{name}.dylib")
      copy "#{name}.dylib", "#{install_dir}/embedded/lib/"
      # Set proper permissions
      command "chmod 755 #{install_dir}/embedded/lib/#{name}.dylib"
      # Update install name for the embedded path (important for macOS)
    else
      raise "macOS dynamic library #{name}.dylib not found in #{project.files_path}/#{name}"
    end
  elsif windows?
    # Copy Windows DLL and import library
    if File.exist?("#{project.files_path}/#{name}/#{name}.dll")
      copy "#{name}.dll", "#{install_dir}/embedded/lib/"
      # Set proper permissions
      command "chmod 755 #{install_dir}/embedded/lib/#{name}.dll"
    else
      raise "Windows DLL #{name}.dll not found in #{project.files_path}/#{name}"
    end
  end

  # Verify the library was copied correctly
  block do
    library_extensions = {
      'linux' => 'so',
      'mac' => 'dylib', 
      'windows' => 'dll'
    }
    
    if linux?
      lib_path = "#{install_dir}/embedded/lib/#{name}.so"
    elsif mac?
      lib_path = "#{install_dir}/embedded/lib/#{name}.dylib"
    elsif windows?
      lib_path = "#{install_dir}/embedded/lib/#{name}.dll"
    end
    
    if not File.exist?(lib_path)
      raise "Failed to install library: #{lib_path} not found after build"
    end
  end
end
