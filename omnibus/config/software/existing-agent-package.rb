name 'existing-agent-package'

description 'A previously built artifact, unpacked'

always_build true

# TODO: Configurable URL, version, and arch
target_package = "datadog-agent_7.64.0-1_arm64.deb"
source url: "https://apt.datadoghq.com/pool/d/da/#{target_package}",
       sha256: "c3d8b4c879530967876ccd315876fc40b8c77e945e44297ff6cb854713bb3dd4",
       target_filename: target_package

build do
  command "dpkg --unpack #{target_package}"
end
