class FIPSSymbolsNotFound < StandardError
end

class FIPSComplianceChecker
    include Omnibus::Logging

    def initialize(symbol)
        @symbol = symbol
      end

    def check(binary_path)
        symbols = `go tool nm #{binary_path}`
        count = symbols.scan(@symbol).count

        if count > 0
            puts "Symbol '#{@symbol}' found #{count} times in binary '#{binary_path}'."
        else
            puts "Unix: Symbols found for #{binary_path}: '#{symbols}'"
            raise "Expected to find '#{@symbol}' symbol in #{binary_path} but did not"
        end
    end
end

