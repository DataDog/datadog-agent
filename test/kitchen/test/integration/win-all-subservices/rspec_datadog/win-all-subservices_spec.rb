require_relative 'spec_helper'

describe 'win-all-subservices' do
  include_examples 'Agent install'
  include_examples 'Basic Agent behavior'
  it_behaves_like 'an Agent with APM enabled'
  it_behaves_like 'an Agent with logs enabled'
  it_behaves_like 'an Agent with process enabled'
  it_behaves_like 'a running Agent with APM'
  it_behaves_like 'a running Agent with process enabled'
  include_examples 'Agent uninstall'
end
