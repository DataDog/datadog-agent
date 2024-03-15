require 'csv'
require 'fileutils'
require 'kernel_out_spec_helper'
require 'open3'
require 'rexml/document'

print KernelOut.format(`cat /etc/os-release`)
print KernelOut.format(`uname -a`)

arch = `uname -m`.strip
release = `uname -r`.strip
osr = Hash[*CSV.read("/etc/os-release", col_sep: "=").flatten(1)]
platform = "#{osr["ID"]}-#{osr["VERSION_ID"]}"

cws_platform = File.read('/tmp/security-agent/cws_platform').strip

GOLANG_TEST_FAILURE = /FAIL:/

def check_output(output, wait_thr, tag="")
  test_failures = []

  puts "Begin Test Output"
  output.each_line do |line|
    stripped_line = line.strip
    puts KernelOut.format(stripped_line, tag)
    test_failures << KernelOut.format(stripped_line, tag) if line =~ GOLANG_TEST_FAILURE
  end
  puts "End Test Output"

  if test_failures.empty? && !wait_thr.value.success?
    test_failures << KernelOut.format("Test command exited with status (#{wait_thr.value.exitstatus}) but no failures were captured.", tag)
  end

  puts "Test Failures"
  puts test_failures.join("\n")
end

shared_examples "passes" do |bundle, env|
  after :context do
    # Combine all the /tmp/pkgjson/#{bundle}.json files into one /tmp/testjson/#{bundle}.json file, which is then used to print failed tests at the end of the functional test Gitlab job
    print KernelOut.format(`find "/tmp/pkgjson/#{bundle}" -maxdepth 1 -type f -path "*.json" -exec cat >"/tmp/testjson/#{bundle}.json" {} +`)
  end

  base_env = {
    "CI"=>"true",
    "DD_SYSTEM_PROBE_BPF_DIR"=>"/tmp/security-agent/ebpf_bytecode",
    "GOVERSION"=>"unknown"
  }
  final_env = base_env.merge(env)

  testsuite_file_path = "/tmp/security-agent/tests/testsuite"
  it "tests" do |ex|
    Dir.chdir(File.dirname(testsuite_file_path)) do
      output_file_name = "#{bundle}-#{platform}-version-#{release}"

      xmlpath = "/tmp/junit/#{bundle}/#{output_file_name}.xml"
      jsonpath = "/tmp/pkgjson/#{bundle}/#{output_file_name}.json"

      # The package name has to be the real path in order to use agent-platform's CODEOWNER parsing downstream
      # The junitfiles are uploaded to the Datadog CI Visibility product, and for downloading
      # The json files are used to print failed tests at the end of the Gitlab job
      #
      # The tests are retried if they fail, but only if less than 5 failed
      # so that we do not retry the whole testsuite in case of a global failure
      gotestsum_test2json_cmd = ["sudo", "-E",
        "/go/bin/gotestsum",
        "--format", "testname",
        "--junitfile", xmlpath,
        "--jsonfile", jsonpath,
        "--rerun-fails=2",
        "--rerun-fails-max-failures=5",
        "--raw-command", "--",
        "/go/bin/test2json", "-t", "-p", "github.com/DataDog/datadog-agent/pkg/security/tests"
      ]

      testsuite_args = ["-status-metrics", "-loglevel=debug", "-test.v", "-test.count=1"]
      if bundle == "docker"
        testsuite_args.concat(["--env", "docker"])
        gotestsum_test2json_cmd.concat(["docker", "exec", "-e", "DD_SYSTEM_PROBE_BPF_DIR=#{final_env["DD_SYSTEM_PROBE_BPF_DIR"]}",
          "docker-testsuite"])
        output_line_tag = "d"
      else
        output_line_tag = "h"

        if bundle == "ad"
          testsuite_args.concat(["-test.run", "TestActivityDump"])
          output_line_tag = "ad"
        end
      end

      gotestsum_test2json_cmd.concat([testsuite_file_path])
      gotestsum_test2json_cmd.concat(testsuite_args)

      Open3.popen2e(final_env, *gotestsum_test2json_cmd) do |_, output, wait_thr|
        check_output(output, wait_thr, output_line_tag)
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

describe "security-agent" do
  after :all do
    print KernelOut.format(`tar -C /tmp/junit -czf /tmp/junit.tar.gz .`)
    print KernelOut.format(`tar -C /tmp/testjson -czf /tmp/testjson.tar.gz .`)
  end

  case cws_platform
  when "host"
    context 'functional tests running directly on host' do
      env = {
        "DD_TESTS_RUNTIME_COMPILED"=>"1",
      }
      include_examples "passes", "host", env
    end
  when "host-fentry"
    context 'functional tests running directly on host (with fentry)' do
      env = {
        "DD_EVENT_MONITORING_CONFIG_EVENT_STREAM_USE_FENTRY" => "true"
      }
      include_examples "passes", "host", env
    end
  when "docker"
    context 'functional test running inside a container' do
      env = {}
      include_examples "passes", "docker", env
    end
  when "docker-fentry"
    context 'functional tests running directly on docker (with fentry)' do
      env = {
        "DD_EVENT_MONITORING_CONFIG_EVENT_STREAM_USE_FENTRY" => "true"
      }
      include_examples "passes", "docker", env
    end
  when "ad"
    context 'activity dump functional test running on dedicated node' do
      env = {
        "DEDICATED_ACTIVITY_DUMP_NODE"=>"1",
        "DD_TESTS_RUNTIME_COMPILED"=>"1",
      }
      include_examples "passes", "ad", env
    end
  else
    raise "no CWS platform provided"
  end
end
