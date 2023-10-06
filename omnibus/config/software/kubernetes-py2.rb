name "kubernetes-py2"
default_version "18.20.0"

dependency "pip2"
dependency "pyyaml-py2"

source :url => "https://files.pythonhosted.org/packages/9c/f8/0eb10c6939b77788c10449d47d85a4740bb4a5608e1a504807fcdb5babd0/kubernetes-#{version}.tar.gz",
       :sha256 => "0c72d00e7883375bd39ae99758425f5e6cb86388417cf7cc84305c211b2192cf",
       :extract => :seven_zip

relative_path "kubernetes-#{version}"

build do
  license "Apache-2.0"
  license_file "./LICENSE.txt"

  command "#{install_dir}/embedded/bin/pip2 install ."
end
