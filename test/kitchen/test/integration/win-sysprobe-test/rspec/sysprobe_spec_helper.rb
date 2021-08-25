require "rspec/core/formatters/base_text_formatter"

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
    @output << "\nstarted #{notification.group.description}\n"
  end

  def example_group_finished(notification)
    @output << "finished #{notification.group.description}\n\n"
  end

  def dump_summary(notification)
    @output << "Finished in #{RSpec::Core::Formatters::Helpers.format_duration(notification.duration)}.\n"
    @output << "Platform: #{`Powershell -C \"Get-WmiObject Win32_OperatingSystem | Select Caption, OSArchitecture, Version, BuildNumber | FL\"`}\n\n"
  end

  def dump_failures(notification) # ExamplesNotification
    if notification.failed_examples.length > 0
      @output << "\n#{RSpec::Core::Formatters::ConsoleCodes.wrap("FAILURES:", :failure)}\n\n"
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
