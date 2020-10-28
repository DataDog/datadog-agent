# Clone the repo; recursive is needed to pick up the submodule inside
git clone --depth 1 --branch 1.2.0 --recursive https://github.com/DataDog/libyajl2-gem libyajl2 
cd libyajl2

# Remove existing rake install, we don't need it and its version is too high
gem uninstall -x rake
# Install dev dependencies - maybe --path should be used to do a local install, but unsure how this will affect the next commands
bundle install
# Prepare the repo
rake prep
# Install the gem
gem build ./libyajl2.gemspec
gem install ./libyajl2-1.2.0.gem

# Cleanup
cd ..
Remove-Item -Recurse -Force libyajl2
