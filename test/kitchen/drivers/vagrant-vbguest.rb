Vagrant.configure("2") do |c|
    if Vagrant.has_plugin?("vagrant-vbguest") then
        c.vbguest.auto_update = false
    end
end

