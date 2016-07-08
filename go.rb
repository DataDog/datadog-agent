

def go_fmt(path)
  out = `go fmt #{path}/...`
  errors = out.split("\n")
  if errors.length > 0
    puts out
    fail
  end
end

def go_lint(path)
  out = `golint #{path}/...`
  errors = out.split("\n")
  puts "#{errors.length} linting issues found"
  if errors.length > 0
    puts out
    fail
  end
end
