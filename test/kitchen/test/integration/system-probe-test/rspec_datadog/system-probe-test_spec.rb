require 'fileutils'
require 'kernel_out_spec_helper'
require 'open3'
require 'csv'
require 'rexml/document'

root_dir = "/tmp/ci/system-probe"
tests_dir = ::File.join(root_dir, "tests")

GOLANG_TEST_FAILURE = /FAIL:/

skip_prebuilt_tests = Array.[](
  "pkg/collector/corechecks/ebpf/probe"
)

runtime_compiled_tests = Array.[](
  "pkg/network/tracer",
  "pkg/network/protocols/http",
  "pkg/collector/corechecks/ebpf/probe"
)

co_re_tests = Array.[](
  "pkg/collector/corechecks/ebpf/probe",
  "pkg/network/protocols/http"
)

TIMEOUTS = {
  "pkg/network/protocols" => "5m",
  # disable timeouts for pkg/network/tracer
  "pkg/network/protocols/http$" => "0",
  "pkg/network/tracer$" => "0",
}

DEFAULT_TIMEOUT = "10m"

def get_timeout(package)
  match_size = 0
  timeout = DEFAULT_TIMEOUT

  # determine longest match
  TIMEOUTS.each do |k, v|
    k = Regexp.new(k)
    v = String(v)

    if package.match?(k) && k.source.size > match_size
      match_size = k.source.size
      timeout = v
    end
  end

  timeout
end

print KernelOut.format(`cat /etc/os-release`)
print KernelOut.format(`uname -a`)

arch = `uname -m`.strip
release = `uname -r`.strip
osr = Hash[*CSV.read("/etc/os-release", col_sep: "=").flatten(1)]
platform = Gem::Platform.local.os
osname = "#{osr["ID"]}-#{osr["VERSION_ID"]}"

##
## The main chef recipe (test\kitchen\site-cookbooks\dd-system-probe-check\recipes\default.rb)
## copies the necessary files (including the precompiled object files), and sets the mode to
## 0755, which causes the test to fail.  The object files are not being built during the
## test, anyway, so set them to the expected value
##
Dir.glob("#{tests_dir}/pkg/ebpf/bytecode/build/*.o").each do |f|
  FileUtils.chmod 0644, f, :verbose => true
end
Dir.glob("#{tests_dir}/pkg/ebpf/bytecode/build/co-re/*.o").each do |f|
  FileUtils.chmod 0644, f, :verbose => true
end

shared_examples "passes" do |bundle, env, filter, filter_inclusive|
  after :context do
    print KernelOut.format(`find "/tmp/pkgjson/#{bundle}" -maxdepth 1 -type f -path "*.json" -exec cat >"/tmp/testjson/#{bundle}.json" {} +`)
  end

  Dir.glob("#{tests_dir}/**/testsuite").sort.each do |f|
    pkg = f.delete_prefix("#{tests_dir}/").delete_suffix('/testsuite')
    next unless (filter_inclusive and filter.include? pkg) or (!filter_inclusive and !filter.include? pkg)

    base_env = {
      "DD_SYSTEM_PROBE_BPF_DIR"=>"#{tests_dir}/pkg/ebpf/bytecode/build",
      "DD_SYSTEM_PROBE_JAVA_DIR"=>"#{tests_dir}/pkg/network/java",
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
          "/go/bin/test2json", "-t", "-p", pkg, f, "-test.v", "-test.count=1", "-test.timeout=#{get_timeout(pkg)}"
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
          props.add_element("property", { "name" => "dd_tags[os.name]", "value" => osname })
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
    include_examples "passes", "prebuilt", env, skip_prebuilt_tests, false
  end

  context "runtime compiled" do
    env = {
      "DD_ENABLE_RUNTIME_COMPILER"=>"true",
      "DD_ALLOW_PRECOMPILED_FALLBACK"=>"false",
      "DD_ENABLE_CO_RE"=>"false"
    }
    include_examples "passes", "runtime", env, runtime_compiled_tests, true
  end

  context "CO-RE" do
    env = {
      "DD_ENABLE_CO_RE"=>"true",
      "DD_ENABLE_RUNTIME_COMPILER"=>"false",
      "DD_ALLOW_RUNTIME_COMPILED_FALLBACK"=>"false",
      "DD_ALLOW_PRECOMPILED_FALLBACK"=>"false"
    }
    include_examples "passes", "co-re", env, co_re_tests, true
  end

  context "fentry" do
    env = {
      "ECS_FARGATE"=>"true",
      "DD_ENABLE_CO_RE"=>"true",
      "DD_ENABLE_RUNTIME_COMPILER"=>"false",
      "DD_ALLOW_RUNTIME_COMPILED_FALLBACK"=>"false"
    }
    if osname == "amzn-2" and arch == "x86_64" and release.start_with?("5.10.")
      include_examples "passes", "fentry", env, skip_prebuilt_tests, false
    end
  end
end
