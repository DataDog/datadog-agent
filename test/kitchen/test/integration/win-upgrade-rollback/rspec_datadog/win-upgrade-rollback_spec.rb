require 'spec_helper'

describe 'win-upgrade-rollback' do
  include_examples 'Agent install'
  include_examples 'Basic Agent behavior'
  it_behaves_like 'an upgraded Agent with the expected version'
  include_examples 'Agent uninstall'
end
