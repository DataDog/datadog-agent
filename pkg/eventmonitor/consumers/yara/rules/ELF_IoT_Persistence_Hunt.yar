rule ELF_IoT_Persistence_Hunt {
    meta:
        description = "Hunts for ELF files with persistence and download capabilities"
        author = "4r4"
        date = "2025-12-25"
        yarahub_uuid = "f906fc31-9621-4d31-83af-bfacb0f9c113"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "87af60a6a0685176e1bfbd101fd0190a"

    strings:
        // ELF Magic Bytes
        $elf = { 7F 45 4C 46 }

        // Persistence indicators (Startup scripts)
        $p1 = "/etc/init.d"
        $p2 = "/etc/rc.d"
        $p3 = "/S99" 
        $p4 = "chkconfig"

        // Downloader & Execution tools
        $d1 = "wget"
        $d2 = "curl"
        $d3 = "tftp"
        $d4 = "/bin/sh"

    condition:
        $elf at 0 
        and any of ($p*) 
        and any of ($d*)
        and filesize < 500KB
}