require 'fileutils'
require 'kernel_out_spec_helper'
require 'open3'
require 'csv'
require 'rexml/document'

root_dir = "/tmp/ci/system-probe"
tests_dir = ::File.join(root_dir, "tests")

GOLANG_TEST_FAILURE = /FAIL:/

TIMEOUTS = {
  "pkg/network/protocols/http$" => "15m",
  "pkg/network/tracer$" => "55m",
  "pkg/network/usm$" => "55m",
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

describe "system-probe" do
  after :all do
    print KernelOut.format(`find "/tmp/pkgjson" -maxdepth 1 -type f -path "*.json" -exec cat >"/tmp/testjson/out.json" {} +`)
    print KernelOut.format(`tar -C /tmp/junit -czf /tmp/junit.tar.gz .`)
    print KernelOut.format(`tar -C /tmp/testjson -czf /tmp/testjson.tar.gz .`)
  end

  Dir.glob("#{tests_dir}/**/testsuite").sort.each do |f|
    pkg = f.delete_prefix("#{tests_dir}/").delete_suffix('/testsuite')
    final_env = {
      "DD_SYSTEM_PROBE_BPF_DIR"=>"#{tests_dir}/pkg/ebpf/bytecode/build",
      "DD_SYSTEM_PROBE_JAVA_DIR"=>"#{tests_dir}/pkg/network/protocols/tls/java",
      "GOVERSION"=>"unknown",
      # force color support to be detected
      "GITLAB_CI"=>"true",
    }
    junitfile = pkg.gsub("/","-") + ".xml"

    it "#{pkg} tests" do |ex|
      Dir.chdir(File.dirname(f)) do
        xmlpath = "/tmp/junit/#{junitfile}"
        cmd = ["sudo", "-E",
          "/go/bin/gotestsum",
          "--format", "dots",
          "--junitfile", xmlpath,
          "--jsonfile", "/tmp/pkgjson/#{pkg.gsub("/","-")}.json",
          "--rerun-fails=2",
          "--rerun-fails-max-failures=100",
          "--raw-command", "--",
          "/go/bin/test2json", "-t", "-p", pkg, f, "-test.v", "-test.count=1", "-test.timeout=#{get_timeout(pkg)}"
        ]

        Open3.popen2e(final_env, *cmd) do |_, output, wait_thr|
          output.each_line do |line|
            puts KernelOut.format(line.strip)
          end
        end

        xmldoc = REXML::Document.new(File.read(xmlpath))
        REXML::XPath.each(xmldoc, "//testsuites/testsuite/properties") do |props|
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
