require 'find'
require 'singleton'

class ForbiddenSymbolsFoundError < StandardError

end

# Helper class to locate `dumpbin.exe` on Windows
class Dumpbin
  include Singleton
  include Omnibus::Logging

  def path
    if !@dumpbin
      vsroot = ENV["VCINSTALLDIR"]
      if !vsroot
        log.warn(self.class.name) { "VC not detected in environment; checking other locations" }
        # VSTUDIO_ROOT is a Datadog env var, it should be present
        vsroot = ENV["VSTUDIO_ROOT"]
        if !vsroot
          log.error(self.class.name) { "Could not find a Visual Studio installation" }
          raise
        end
        # VSTUDIO_ROOT is not defined with "VC" in its path, so append it
        vsroot += "\\VC\\"
      end
      Find.find(vsroot) do |path|
        if File.basename(path) == "dumpbin.exe"
          @dumpbin = path
          Find.prune
        else
          next
        end
      end
      if !@dumpbin
        log.error(self.class.name) { "Could not find dumpbin.exe" }
        raise
      end
    end
    @dumpbin
  end

  def call(binary_path)
    `"#{path()}" /SYMBOLS "#{binary_path}"`.encode("UTF-8", invalid: :replace)
  end
end

# The VisualStudioSymbolsChecker can be used to inspect
# the symbols in Win32 binaries using `dumpbin.exe`
# included in Visual Studio
class VisualStudioSymbolsInspector
  include Omnibus::Logging

  def initialize(binary, &block)
    @binary = binary
    @block = block
  end

  def inspect()
    log.info(self.class.name) { "Inspecting binary #{@binary}" }
    @block.call(Dumpbin.instance.call(@binary))
  end
end

# The GoSymbolChecker class can be used to inspect
# the symbols in Go binaries using the go tool `nm`
class GoSymbolsInspector
  include Omnibus::Logging

  def initialize(binary, &block)
    @binary = binary
    @block = block
  end

  def inspect()
    log.info(self.class.name) { "Inspecting binary #{@binary}" }
    @block.call(`go tool nm #{@binary}`)
  end
end