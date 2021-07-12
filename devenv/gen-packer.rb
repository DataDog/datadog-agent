require 'erb'

class Builder

    def initialize(type)
        if type == "client"
            @name = "windows_10_ent"
            @autounattend = "./answer_files/10_Ent/Autounattend.xml"
            @isourl = "https://software-download.microsoft.com/download/pr/18362.30.190401-1528.19h1_release_svc_refresh_CLIENTENTERPRISEEVAL_OEMRET_x64FRE_en-us.iso"
            @checksum = "ab4862ba7d1644c27f27516d24cb21e6b39234eb3301e5f1fb365a78b22f79b3"
        else
            @name = "windows_2019"
            @autounattend = "./answer_files/2019_core/Autounattend.xml"
            @isourl = "https://software-download.microsoft.com/download/pr/17763.379.190312-0539.rs5_release_svc_refresh_SERVER_EVAL_x64FRE_en-us.iso"
            @checksum = "221F9ACBC727297A56674A0F1722B8AC7B6E840B4E1FFBDD538A9ED0DA823562"
        end
    end
    def ostype
        raise "Not implemented"
    end
    def vmtype
        raise "Not implemented"
    end
    def isourl
        @isourl
    end
    def checksum
        @checksum
    end
    def autounattend
        @autounattend
    end
end

class VMWare < Builder
    def initialize(type)
        super(type)
        if type == "client"
            @ostype = "windows9-64"
        else
            @ostype = "windows9srv-64"
        end
    end
    def ostype
        @ostype
    end
    def vmtype
        "vmware-iso"
    end
end

class Parallels < Builder
    def initialize(type)
        super(type)
        if type == "client"
            @ostype = "win-10"
        else
            @ostype = "win-2019"
        end
    end
    def ostype
        @ostype
    end
    def vmtype
        "parallels-iso"
    end
end

class Virtualbox < Builder
    def initialize(type)
        super(type)
        if type == "client"
            @ostype = "Windows10_64"
        else
            @ostype = "Windows2016_64"
        end
    end
    def ostype
        @ostype
    end
    def vmtype
        "virtualbox-iso"
    end
end

def build(name, type)
    template = ERB.new(File.read('packer.json.erb'))

    print template.result_with_hash(
        name: name,
        vagrantfile: "Vagrantfile.template",
        builders: [
            Parallels.new(type),
            VMWare.new(type),
            Virtualbox.new(type)
        ])
end