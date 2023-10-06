require "rspec/core/formatters/base_text_formatter"

COLORS = [
  :green,
  :blue,
  :magenta,
  :yellow,
  :cyan,
]

class KernelOut
  @@release = `uname -r`.strip
  if File.exist?('/tmp/color_idx')
    color_idx = File.read('/tmp/color_idx').strip.to_i - 1
    @@color = COLORS[color_idx]
  else
    @@color = :no_format
  end

  def self.format(text, tag="")
    tag = "[#{tag}]" if tag != ""
    if @@color != :no_format
      return RSpec::Core::Formatters::ConsoleCodes.wrap("[#{@@release}]#{tag} #{text}", @@color)
    else
      return "[#{@@release}]#{tag} #{text}"
    end
  end
end

class CustomFormatter
  RSpec::Core::Formatters.register self, :example_passed, :example_failed, :dump_summary, :dump_failures, :example_group_started, :example_group_finished

  def initialize(output)
    @output = output
  end

  # Remove "."'s from the test execution output
  def example_passed(_)
  end

  # Remove "F"'s from the test execution output
  def example_failed(_)
  end

  def example_group_started(notification)
    @output << "\n"
    @output << KernelOut.format("started #{notification.group.description}\n")
  end

  def example_group_finished(notification)
    @output << KernelOut.format("finished #{notification.group.description}\n\n")
  end

  def dump_summary(notification)
    @output << KernelOut.format("Finished in #{RSpec::Core::Formatters::Helpers.format_duration(notification.duration)}.\n")
    @output << KernelOut.format("#{notification.totals_line}\n")
    @output << KernelOut.format("Platform: #{`uname -a`}\n\n")
  end

  def dump_failures(notification) # ExamplesNotification
    if notification.failed_examples.length > 0
      rel = KernelOut.format("")
      failures = RSpec::Core::Formatters::ConsoleCodes.wrap("FAILURES:", :failure)
      @output << "\n#{rel} #{failures}\n\n"
      @output << error_summary(notification)
    end
  end

  private

  def error_summary(notification)
    summary_output = notification.failed_examples.map do |example|
      "#{example.full_description}:\n#{example.execution_result.exception.message}\n\n"
    end

    summary_output.join
  end
end


RSpec.configure do |config|
  config.formatter = CustomFormatter
end
