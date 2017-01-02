

def go_fmt(path, fail_on_mod)
  out = `go fmt #{path}/...`
  errors = out.split("\n")
  if errors.length > 0
    puts "Reformatted the following files:"
    puts out
    fail if fail_on_mod
  end
end

def go_lint(path)
  out = `golint #{path}/...`
  errors = out.split("\n")
  puts "#{errors.length} linting issues found in #{path}"
  if errors.length > 0
    puts out
    fail
  end
end

def go_vet(path)
  out = `go vet #{path}/... 2>&1`
  errors = out.split("\n")
  puts "vet found #{errors.length} issues in #{path}"
  if errors.length > 0
    puts out
    fail
  end
end