rule ELF_Toriilike_persist {
    meta:
        description = "Detects Torii IoT Botnet (stealthier Mirai alternative)"
        author = "4r4"
        reference = "Identified via researched data"
        date = "2025-12-25"
        family = "Torii"
        
        // Hashes for reference
        sha256 = "f970290ff41ba899fedea4999e76461860b7cdab86a1847193302edb0ee691ba"
        
        // YARAify Specific Fields
        yarahub_uuid = "031cfb94-1972-4c6c-b1a5-9f2d6b670836"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "87af60a6a0685176e1bfbd101fd0190a"

    strings:
        $elf_header = { 7F 45 4C 46 }

        // unique random seed string observed in Torii/Persirai samples
        $unique_seed = "npxXoudifFeEgGaACScs"
        
        // persistence Mechanisms (Torii allows reboot survival)
        $persist_path = "/S99systemjob"
        $persist_cmd  = "@reboot %s"

    condition:
        $elf_header at 0 and 
        (
            $unique_seed or 
            ($persist_path and $persist_cmd)
        )
}