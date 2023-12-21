name "lmdb"
default_version "0.9.29"

source :url => "https://github.com/LMDB/lmdb/archive/LMDB_#{version}.tar.gz",
       :sha256 => "22054926b426c66d8f2bc22071365df6e35f3aacf19ad943bc6167d4cae3bebb",
       :extract => :seven_zip

relative_path "lmdb-LMDB_#{version}/libraries/liblmdb"

build do
    license "OpenLDAP Public License"
    license_file "https://raw.githubusercontent.com/LMDB/lmdb/LMDB_#{version}/libraries/liblmdb/COPYRIGHT"
    patch source: "allow-makefile-override-vars.diff"
    env = with_standard_compiler_flags(with_embedded_path)
    env["prefix"] = "#{install_dir}/embedded/"
    env["XCFLAGS"] = env["CFLAGS"]

    # https://www.linuxfromscratch.org/blfs/view/8.3/server/lmdb.html
    command "make -j #{workers}", :env => env
    if mac_os_x?
        # MacOS' sed requires `-i ''` rather than just `-i`
        command "sed -i '' 's| liblmdb.a||' Makefile", :env => env
    else
        command "sed -i 's| liblmdb.a||' Makefile", :env => env
    end
    command "make install", :env => env

end
