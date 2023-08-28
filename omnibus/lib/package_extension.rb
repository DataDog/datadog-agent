class BannedSymbolsChecker
    class ForbiddenSymbolsFoundError < StandardError

    end

    def initialize(binary, patterns)
        @binary = binary
        @patterns = patterns
    end

    def check()
        count = `inv check-symbols --patterns="#{@patterns}" --binary-path="#{@binary}"`.strip
        if $?.exitstatus != 0
            raise RuntimeError.new("Invoking check-symbols failed, process exited with status: #{$?.exitstatus}")
        end

        if count.to_i > 0
            raise ForbiddenSymbolsFoundError.new("A forbidden symbol was found")
        end
    end
end
  

module Omnibus
    class Packager::MSI
        class << self
            def build(&block)
                if block
                @build = -> {
                    if banned_symbols
                        puts "Checking for banned symbols"
                        banned_symbols.each do |symbols_pattern|
                            command 'inv check-symbols --patterns="glog.init" --binary-path=".\bin\agent\agent.exe"'
                            puts "signing #{signfile}"
                            sign_package(signfile)
                        end
                    end
                } >> block
                else
                    @build
                end
            end
        end

        #
        # Set or retrieve a list of symbols that should not be found in the binaries
        #
        def banned_symbols(val = NULL)
            unless null?(val)
                unless val.is_a?(Hash)
                  raise InvalidValue.new(:banned_symbols,
                                         "be a kind of `Array', but was `#{val.class.inspect}'")
                end
            end
            @banned_symbols = val
          end
          expose :banned_symbols

end