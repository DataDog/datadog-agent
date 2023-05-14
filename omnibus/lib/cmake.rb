def cmake(*args)
    cwd = "#{project_dir}/build"
    options = args.last.is_a?(Hash) ? args.pop : {}
    options = { cwd: cwd }.merge(options)

    mkdir "build"

    cmake_cmd = ["cmake", ".."]
    prefix = options.delete(:prefix) || "#{install_dir}/embedded"
    cmake_cmd << "-DCMAKE_INSTALL_PREFIX=#{prefix}" if prefix && prefix != ""
    libdir = options.delete(:libdir) || "lib"
    cmake_cmd << "-DCMAKE_INSTALL_LIBDIR=#{libdir}" if libdir && libdir != ""
    rpath = options.delete(:rpath) || "#{prefix}/#{libdir}"
    cmake_cmd << "-DCMAKE_INSTALL_RPATH=#{rpath}" if rpath && rpath != ""
    cmake_cmd.concat args
    cmake_cmd = cmake_cmd.join(" ").strip

    command(cmake_cmd, options)

    make("-j #{workers}", options)
    make("install", options)
end

