require 'spec_helper'

describe 'dd-agent' do
  include_examples 'Agent'
end


if os == :windows
  describe 'system-files' do

    before_files = File.readlines('c:/before-files.txt')
    after_files = File.readlines('c:/after-files.txt')

    missing_files = before_files - after_files
    new_files = after_files - before_files

    puts "New files:"
    new_files.each { |f| puts(f) }

    puts "Missing files:"
    missing_files.each { |f| puts(f) }

    it "didn't remove any previously present files" do
      expect(missing_files).to be_empty
    end

  end
end
