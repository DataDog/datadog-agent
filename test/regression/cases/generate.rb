#!/usr/bin/env ruby

COUNT = 10000

# Get tag count from command line argument
if ARGV.length != 1
  puts "Usage: #{$0} <tag_count>"
  puts "Example: #{$0} 20"
  exit 1
end

tag_count = ARGV[0].to_i

if tag_count <= 0
  puts "Error: tag_count must be a positive integer"
  exit 1
end

# Generate all possible tags with pattern tag{a-z}{a-z}
all_tags = ('a'..'z').flat_map do |a|
  ('a'..'z').map { |b| "tag#{a}#{b}" }
end

# Take only the first tag_count tags
selected_tags = all_tags.take(tag_count)

puts "metric_tag_filterlist:"

COUNT.times do |i|
  tags = selected_tags.map do |name|
    "  - #{name}"
  end.join("\n")

  puts "- metric_name: metric#{i}"
  puts "  action: include"
  puts "  tags:"
  puts "#{tags}"
end
