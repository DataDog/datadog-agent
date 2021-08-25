require 'spec_helper'

describe 'dd-agent-step-by-step' do
  include_examples 'Agent install'
  include_examples 'Agent behavior'
  include_examples 'Agent uninstall'
end
