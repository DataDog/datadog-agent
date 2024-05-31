require File.expand_path('../boot', __FILE__)

Bundler.require(:default, :assets, Rails.env)

module MyHTTPRailsAppX
  class Application < Rails::Application
  end
end
