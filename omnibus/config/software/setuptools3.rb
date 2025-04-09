name "setuptools3"

# The version of setuptools used must be at least equal to the one bundled with the Python version we use
# Python 3.8.16 bundles setuptools 56.0.0
default_version "66.1.1"

skip_transitive_dependency_licensing true

dependency "pip3"

wheel_filename = "setuptools-#{version}-py3-none-any.whl"

source :url => "https://files.pythonhosted.org/packages/c2/8b/abb577ca6ab2c71814d535b1ed1464c5f4aaefe1a31bbeb85013eb9b2401/#{wheel_filename}",
       :sha256 => "6f590d76b713d5de4e49fe4fbca24474469f53c83632d5d0fd056f7ff7e8112b",
       :target_filename => wheel_filename

build do
  # 2.0 is the license version here, not the python version
  license "Python-2.0"

  if ohai["platform"] == "windows"
    python = "#{windows_safe_path(python_3_embedded)}\\python.exe"
  else
    python = "#{install_dir}/embedded/bin/python3"
  end

  command "#{python} -m pip install ./#{wheel_filename}"

  if ohai["platform"] != "windows"
    block do
      FileUtils.rm_f(Dir.glob("#{install_dir}/embedded/lib/python3.*/site-packages/setuptools/*.exe"))
    end
  end
end
