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

  Dir.glob('/tmp/security-agent/**/testsuite').each do |f|
    pkg = f.delete_prefix('/tmp/security-agent/').delete_suffix('/testsuite')

    base_env = {
      "DD_SYSTEM_PROBE_BPF_DIR"=>"/tmp/security-agent/ebpf_bytecode",
      "GOVERSION"=>"unknown"
    }
    junitfile = pkg.gsub("/","-") + ".xml"

    it "#{pkg} tests" do |ex|
      Dir.chdir(File.dirname(f)) do
        xmlpath = "/tmp/junit/#{bundle}/#{junitfile}"
        cmd = ["sudo", "-E",
          "/go/bin/gotestsum",
          "--format", "dots",
          "--junitfile", xmlpath,
          "--jsonfile", "/tmp/pkgjson/#{bundle}/#{pkg.gsub("/","-")}.json",
          "--raw-command", "--",
          "/go/bin/test2json", "-t", "-p", pkg, f, "-test.v", "-test.count=1"
        ]

        final_env = base_env.merge(env)
        Open3.popen2e(final_env, *cmd) do |_, output, wait_thr|
          output.each_line do |line|
            puts KernelOut.format(line.strip)
          end
        end

        xmldoc = REXML::Document.new(File.read(xmlpath))
        REXML::XPath.each(xmldoc, "//testsuites/testsuite/properties") do |props|
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
end

describe "security-agent" do
  after :all do
    print KernelOut.format(`tar -C /tmp/junit -czf /tmp/junit.tar.gz .`)
    print KernelOut.format(`tar -C /tmp/testjson -czf /tmp/testjson.tar.gz .`)
  end

  case cws_platform
  when "host"
    describe 'functional test running directly on host' do
      it 'successfully runs' do
        env = {}
        include_examples "passes", "host", env
      end
    end
  when "docker"
    describe 'functional test running inside a container' do
      it 'successfully runs' do
        Open3.popen2e("sudo", "docker", "exec", "-e", "DD_SYSTEM_PROBE_BPF_DIR=/tmp/security-agent/ebpf_bytecode", "docker-testsuite", "/tmp/security-agent/testsuite", "-test.v", "-status-metrics", "--env", "docker") do |_, output, wait_thr|
          test_failures = check_output(output, wait_thr, "a")
          expect(test_failures).to be_empty, test_failures.join("\n")
        end
      end
    end
  when "ad"
    describe 'activity dump functional test running on dedicated node' do
      it 'successfully runs' do
        Open3.popen2e({"DEDICATED_ACTIVITY_DUMP_NODE"=>"1", "DD_TESTS_RUNTIME_COMPILED"=>"1", "DD_RUNTIME_SECURITY_CONFIG_RUNTIME_COMPILATION_ENABLED"=>"true", "DD_SYSTEM_PROBE_BPF_DIR"=>"/tmp/security-agent/ebpf_bytecode"}, "sudo", "-E", "/tmp/security-agent/testsuite", "-test.v", "-status-metrics", "-test.run", "TestActivityDump") do |_, output, wait_thr|
          test_failures = check_output(output, wait_thr, "a")
          expect(test_failures).to be_empty, test_failures.join("\n")
        end
      end
    end
  else
    raise "no CWS platform provided"
  end
end
