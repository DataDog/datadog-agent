require 'find'
require 'singleton'

class ForbiddenSymbolsFoundError < StandardError

end

# Helper class to locate `dumpbin.exe` on Windows
class Dumpbin
  include Singleton

  def path
    if !@dumpbin
      vsroot = ENV["VCINSTALLDIR"]
      if !vsroot
        print("VC not detected in environment; checking other locations")
        # VSTUDIO_ROOT is a Datadog env var, it should be present
        vsroot = ENV["VSTUDIO_ROOT"]
        if !vsroot
          raise RuntimeError.new("Could not find a Visual Studio installation")
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
        raise RuntimeError.new("Could not find dumpbin.exe")
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
  def initialize(binary, &block)
    @binary = binary
    @block = block
  end

  def inspect()
    @block.call(Dumpbin.instance.call(@binary))
  end
end

# The GoSymbolChecker class can be used to inspect
# the symbols in Go binaries using the go tool `nm`
class GoSymbolsInspector
  def initialize(binary, &block)
    @binary = binary
    @block = block
  end

  def inspect()
    @block.call(`go tool nm #{@binary}`)
  end
end