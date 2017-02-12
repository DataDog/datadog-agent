name "conda"
description "none provided"

source :url => "https://gist.githubusercontent.com/masci/6351be014f6950e0f9918c3337034e40/raw/537a53f0ac072c180eca8e475fb6f3ef5bd735e5/datadog-agent.yaml",
       :md5 => "8eff18e2e9fcb8e7c48200b7372face1"

build do 
    # create the env
    command "conda env create --force -q -f datadog-agent.yaml"
end
