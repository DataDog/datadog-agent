name "libcom_err"
default_version "1.47.0"

# We will download and build e2fsprogs, then move libcom_err.so files into the correct library
source :url => "https://mirrors.edge.kernel.org/pub/linux/kernel/people/tytso/e2fsprogs/v#{version}/e2fsprogs-#{version}.tar.gz",
       :sha256 => "0b4fe723d779b0927fb83c9ae709bc7b40f66d7df36433bef143e41c54257084",
       :extract => :seven_zip

relative_path "e2fsprogs-#{version}"

build do
  license = "MIT"
  license_file = "https://git.kernel.org/pub/scm/fs/ext2/e2fsprogs.git/tree/lib/et/com_err.c"

  env = with_standard_compiler_flags(with_embedded_path)

  # For libcom_err, we need to build e2fsprogs (since libcom_err is a subpackage of it),
  # and manually move the contents of libcom_err into the Agent
  # Build e2fsprogs in a temp directory
  configure_options = [
    "--enable-elf-shlibs"
  ]
  configure(*configure_options, prefix: "#{install_dir}/embedded/temp_dir", :env => env)
  command "make", :env => env

  # Move libcom_err files directly
  copy "lib/et/libcom_err.so.2.1", "#{install_dir}/embedded/lib/"
  link "#{install_dir}/embedded/lib/libcom_err.so.2.1", "#{install_dir}/embedded/lib/libcom_err.so.2"

  copy "lib/libcom_err.a", "#{install_dir}/embedded/lib/"

  copy "lib/et/com_err.pc", "#{install_dir}/embedded/lib/"

  copy "lib/et/com_err.h", "#{install_dir}/embedded/include/"

  copy "lib/et/compile_et", "#{install_dir}/embedded/bin/"

  # Remove the temp_dir
  delete "#{install_dir}/embedded/temp_dir"
end
