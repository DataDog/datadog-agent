require 'spec_helper'

describe 'win-repair' do
    it_behaves_like 'an installed Agent'
    it_behaves_like 'a running Agent with no errors'
  end
