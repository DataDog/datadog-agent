require_relative 'spec_helper'



describe 'win-all-subservices' do
  it_behaves_like 'an installed Agent'
  it_behaves_like 'an Agent with APM enabled'
  it_behaves_like 'an Agent with logs enabled'
  it_behaves_like 'an Agent with process enabled'
  it_behaves_like 'a running Agent with no errors'
  it_behaves_like 'a running Agent with APM'
  it_behaves_like 'a running Agent with process enabled'
end
  