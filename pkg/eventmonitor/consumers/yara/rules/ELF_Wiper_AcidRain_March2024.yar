
rule ELF_Wiper_AcidRain_March2024 {
    meta:
        Description = "Detects the Acid Rain Wiper Malware"
        author = "Yashraj Solanki - Cyber Threat Intelligence Analyst at Bridewell"
        Credits = "@ShanHolo for sharing the malware file hash and key characteristics"
        Reference = "https://twitter.com/ShanHolo/status/1770083206773002267"
        File_Hash = "6a8824048417abe156a16455b8e29170f8347312894fde2aabe644c4995d7728"
        date = "2024-03-20"
        yarahub_author_twitter = "@RustyNoob619"
        yarahub_reference_md5 = "1bde1e4ecc8a85cffef1cd4e5379aa44"
        yarahub_uuid = "b7279bbd-4112-44fd-8696-ba837096518e"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        malpedia_family = "win.acridrain"

    strings:
        $dev1 = "/dev/sdXX" fullword ascii
        $dev2 = "/dev/null" fullword ascii
        $dev3 = "/dev/dm-XX" fullword ascii
        $dev4 = "/dev/block/mtdblockXX" fullword ascii
        $dev5 = "/dev/mtdblockXX" fullword ascii
        $dev6 = "/dev/mmcblkXX" fullword ascii
        $dev7 = "/dev/ubiXX" fullword ascii
        $dev8 = "/dev/loopXX" fullword ascii
        $dev9 = "/dev/block/mmcblkXX" fullword ascii
        $dev10 = "/dev/mtdXX" fullword ascii
        $usr1 = "/usr/sbin/reboot" fullword ascii
        $usr2 = "/usr/bin/reboot" fullword ascii
        $proc = "/proc/self/exe" fullword ascii

    condition:
        uint32be(0) == 0x7f454c46 //ELF Header
        and $proc
        and 1 of ($usr*) 
        and 3 of ($dev*)
 }