require 'spec_helper'

describe 'the upgraded agent' do
  include_examples 'Agent install'
  include_examples 'Agent behavior'
  it_behaves_like 'an upgraded agent with expected version'

  include_examples 'Agent uninstall'
end
