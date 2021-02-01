require 'spec_helper'

describe 'the agent installed with the npm option' do
    it_behaves_like 'an installed Agent'
    it_behaves_like 'a running Agent with no errors'
    it_behaves_like 'a Windows Agent with NPM driver installed'
  end