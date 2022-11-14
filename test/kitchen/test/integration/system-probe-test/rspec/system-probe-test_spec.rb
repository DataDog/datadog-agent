require 'fileutils'
require 'kernel_out_spec_helper'
require 'open3'
require 'csv'
require 'rexml/document'

GOLANG_TEST_FAILURE = /FAIL:/

runtime_compiled_tests = Array.[](
  "pkg/network/tracer",
  "pkg/network/protocols/http",
)

co_re_tests = Array.[](
  "pkg/collector/corechecks/ebpf/probe"
)

print KernelOut.format(`cat /etc/os-release`)
print KernelOut.format(`uname -a`)

arch = `uname -m`.strip
release = `uname -r`.strip
osr = Hash[*CSV.read("/etc/os-release", col_sep: "=").flatten(1)]
platform = "#{osr["ID"]}-#{osr["VERSION_ID"]}"

##
## The main chef recipe (test\kitchen\site-cookbooks\dd-system-probe-check\recipes\default.rb)
## copies the necessary files (including the precompiled object files), and sets the mode to
## 0755, which causes the test to fail.  The object files are not being built during the
## test, anyway, so set them to the expected value
##
Dir.glob('/tmp/system-probe-tests/pkg/ebpf/bytecode/build/*.o').each do |f|
  FileUtils.chmod 0644, f, :verbose => true
end
Dir.glob('/tmp/system-probe-tests/pkg/ebpf/bytecode/build/co-re/*.o').each do |f|
  FileUtils.chmod 0644, f, :verbose => true
end

shared_examples "passes" do |bundle, env, filter|
  after :context do
    print KernelOut.format(`find "/tmp/pkgjson/#{bundle}" -maxdepth 1 -type f -path "*.json" -exec cat >"/tmp/testjson/#{bundle}.json" {} +`)
  end

  Dir.glob('/tmp/system-probe-tests/**/testsuite').each do |f|
    pkg = f.delete_prefix('/tmp/system-probe-tests/').delete_suffix('/testsuite')
    next unless filter.nil? or filter.include? pkg

    base_env = {
      "DD_SYSTEM_PROBE_BPF_DIR"=>"/tmp/system-probe-tests/pkg/ebpf/bytecode/build",
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

describe "system-probe" do
  after :all do
    print KernelOut.format(`tar -C /tmp/junit -czf /tmp/junit.tar.gz .`)
    print KernelOut.format(`tar -C /tmp/testjson -czf /tmp/testjson.tar.gz .`)
  end

  context "prebuilt" do
    env = {
      "DD_ENABLE_RUNTIME_COMPILER"=>"false",
      "DD_ENABLE_CO_RE"=>"false"
    }
    include_examples "passes", "prebuilt", env
  end

  context "runtime compiled" do
    env = {
      "DD_ENABLE_RUNTIME_COMPILER"=>"true",
      "DD_ALLOW_PRECOMPILED_FALLBACK"=>"false",
      "DD_ENABLE_CO_RE"=>"false"
    }
    include_examples "passes", "runtime", env, runtime_compiled_tests
  end

  context "CO-RE" do
    env = {
      "DD_ENABLE_CO_RE"=>"true",
      "DD_ENABLE_RUNTIME_COMPILER"=>"false",
      "DD_ALLOW_RUNTIME_COMPILED_FALLBACK"=>"false"
    }
    include_examples "passes", "co-re", env, co_re_tests
  end
end
