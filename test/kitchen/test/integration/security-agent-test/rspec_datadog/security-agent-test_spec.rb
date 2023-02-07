require 'csv'
require 'fileutils'
require 'kernel_out_spec_helper'
require 'open3'
require 'rexml/document'

GOLANG_TEST_FAILURE = /FAIL:/

print KernelOut.format(`cat /etc/os-release`)
print KernelOut.format(`uname -a`)

arch = `uname -m`.strip
release = `uname -r`.strip
osr = Hash[*CSV.read("/etc/os-release", col_sep: "=").flatten(1)]
platform = "#{osr["ID"]}-#{osr["VERSION_ID"]}"

cws_platform = File.read('/tmp/security-agent/cws_platform').strip

def check_output(output, wait_thr, tag="")
  test_failures = []

  output.each_line do |line|
    striped_line = line.strip
    puts KernelOut.format(striped_line, tag)
    test_failures << KernelOut.format(striped_line, tag) if line =~ GOLANG_TEST_FAILURE
  end

  if test_failures.empty? && !wait_thr.value.success?
    test_failures << KernelOut.format("Test command exited with status (#{wait_thr.value.exitstatus}) but no failures were captured.", tag)
  end

  test_failures
end

shared_examples "passes" do |bundle, env|
  after :context do
    print KernelOut.format(`find "/tmp/pkgjson/#{bundle}" -maxdepth 1 -type f -path "*.json" -exec cat >"/tmp/testjson/#{bundle}.json" {} +`)
  end

  output_file_name = "#{bundle}-#{platform}-version-#{release}"

  base_env = {
    "DD_SYSTEM_PROBE_BPF_DIR"=>"/tmp/security-agent/ebpf_bytecode",
    "GOVERSION"=>"unknown"
  }
  junitfile = "#{output_file_name}.xml"
  testsuite_file_path = "/tmp/security-agent/tests/testsuite"

  it "tests" do |ex|
    Dir.chdir(File.dirname(testsuite_file_path)) do
      xmlpath = "/tmp/junit/#{bundle}/#{junitfile}"

      if bundle != "docker"
        cmd = ["sudo", "-E",
          "/go/bin/gotestsum",
          "--format", "dots",
          "--junitfile", xmlpath,
          "--jsonfile", "/tmp/pkgjson/#{output_file_name}.json",
          "--raw-command", "--",
          "/go/bin/test2json", "-t", testsuite_file_path, "-test.v", "-test.count=1", "-status-metrics"
        ]

        if bundle == "ad"
          cmd.append(["-test.run", "TestActivityDump"])
        end
      else
        cmd = ["sudo", "docker", "exec", "-e", "docker-testsuite",
          "/go/bin/gotestsum",
          "--format", "dots",
          "--junitfile", xmlpath,
          "--jsonfile", "/tmp/pkgjson/#{output_file_name}.json",
          "--raw-command", "--",
          "/go/bin/test2json", "-t", testsuite_file_path, "-test.v", "-test.count=1", "-status-metrics", "--env", "docker"
        ]
      end

      # TODO (Celia Yuen): delete once SecAgent CI visibility is polished
      puts cmd
      puts env

      final_env = base_env.merge(env)
      Open3.popen2e(final_env, *cmd) do |_, output, wait_thr|
        output.each_line do |line|
          puts KernelOut.format(line.strip)
        end
      end

      xmldoc = REXML::Document.new(File.read(xmlpath))
      REXML::XPath.each(xmldoc, "//testsuite/properties") do |props|
        props.add_element("property", { "name" => "dd_tags[test.bundle]", "value" => bundle })
        props.add_element("property", { "name" => "dd_tags[os.platform]", "value" => platform })
        props.add_element("property", { "name" => "dd_tags[os.architecture]", "value" => arch })
        props.add_element("property", { "name" => "dd_tags[os.version]", "value" => release })
      end
      File.open(xmlpath, "w") do |f|
        xmldoc.write(:output => f, :indent => 4)
      end
    end
  end
end

describe "security-agent" do
  after :all do
    print KernelOut.format(`tar -C /tmp/junit -czf /tmp/junit.tar.gz .`)
    print KernelOut.format(`tar -C /tmp/testjson -czf /tmp/testjson.tar.gz .`)
  end

  case cws_platform
  when "host"
    context 'functional tests running directly on host' do
      env = {}
      include_examples "passes", "host", env
    end
  when "docker"
    context 'functional test running inside a container' do
      env = {}
      include_examples "passes", "docker", env
    end
  when "ad"
    context 'activity dump functional test running on dedicated node' do
      env = {
        "DEDICATED_ACTIVITY_DUMP_NODE"=>"1",
        "DD_TESTS_RUNTIME_COMPILED"=>"1",
        "DD_RUNTIME_SECURITY_CONFIG_RUNTIME_COMPILATION_ENABLED"=>"true"
      }
      include_examples "passes", "ad", env
    end
  else
    raise "no CWS platform provided"
  end
end
