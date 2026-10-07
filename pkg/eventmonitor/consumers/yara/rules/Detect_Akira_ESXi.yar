rule Detect_Akira_ESXi{
    meta:
        author = "Nguyen Thien Long"
        date = "2026-06-08"
        description = "Detects Akira ESXi Variant"
        tlp = "WHITE"
        yarahub_uuid = "e81b83f5-fe4b-4914-b3b0-08bc3508d017" 
        yarahub_reference_md5 = "8ef468f21842ee03e1c5a41a6fef6bba" 
        yarahub_license = "CC0 1.0" 
        yarahub_rule_matching_tlp = "TLP:WHITE" 
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        $elf = { 7F 45 4C 46 }

        //links
        $Akira_blog = "https://akiral2iz6a7qgd3ayp3l6yub7xx2uep76idk3u2kollpj5z3z636bad.onion" ascii
        $Akira_chat_room = "https://akiralkzxzq2dsrzsrvbr2xgbbu2wgsmxryd4csgfameg52n7efvr2id.onion" ascii

        //shell cmds
        $cmd_1 = "esxcli system syslog config set" ascii
        $cmd_2 = "esxcli system coredump file set --unconfigure" ascii
        
        //args
        $arg_1 = "--encryption_percent" ascii
        $arg_2 = "--encryption_path" ascii
        $arg_3 = "--exclude" ascii
        $arg_4 = "--share-file" ascii

        //after attack
        $note_name = "akira_readme.txt" ascii nocase
        $log_fmt = "Log-%d-%m-%Y-%H-%M-%S" ascii nocase

    condition:
        $elf at 0 and
        $Akira_blog and
        (
            $Akira_chat_room or 
            2 of ($cmd_*, $arg_*, $note_name, $log_fmt)
        )
}
