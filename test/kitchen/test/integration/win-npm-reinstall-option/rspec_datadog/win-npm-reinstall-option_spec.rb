require 'spec_helper'
require 'windows_npm_spec_helper'

describe 'the agent installed with no npm options' do
  it_behaves_like 'an installed Agent'
  it_behaves_like 'a running Agent with no errors'
  it_behaves_like 'a Windows Agent with NPM driver installed'
  it_behaves_like 'a Windows Agent with closed source enabled'
  it_behaves_like 'a Windows Agent with NPM running'
end
