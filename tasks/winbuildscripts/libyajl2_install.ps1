pushd .
New-Item -Force -Path c:\tmp -ItemType Directory
cd \tmp

# Clone the repo; recursive is needed to pick up the submodule inside
git clone --depth 1 --branch 1.2.0 --recursive https://github.com/DataDog/libyajl2-gem libyajl2 
cd libyajl2

# Remove existing rake install, we don't need it and its version is too high
gem uninstall -x rake

# We don't need the development_extras group, we're not running tests
bundle config set --local without 'development_extras'

# Install dev dependencies - maybe --path should be used to do a local install, but unsure how this will affect the next commands
bundle install

# Prepare the repo
rake prep
# Install the gem
gem build ./libyajl2.gemspec
gem install ./libyajl2-1.2.0.gem

# Cleanup
popd
Remove-Item -Recurse -Force c:\tmp\libyajl2