rule IDATDropper {
    meta:
        author = "NDA0E"
        yarahub_author_twitter = "@NDA0E"
        date = "2024-07-30"
        description = "Detects files containing embedded JavaScript; the JS executes a PowerShell command which either downloads IDATLoader in an archive, or an executable (not IDATLoader) which is loaded into memory. The modified PE will only run if it's executed as an HTML Application (.hta)."
        yarahub_uuid = "9dbff40b-6257-438d-8932-e7fb652a4d6a"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "db1ae063d1be2bcb6af8f4afb145cdc4"
        yarahub_reference_link = "https://cyble.com/blog/increase-in-the-exploitation-of-microsoft-smartscreen-vulnerability-cve-2024-21412/"
        malpedia_family = "win.emmenhtal"
    
    strings:
        $hta = "HTA:APPLICATION" ascii
        
        $script_start = "<script>" ascii
        $variable = "var " ascii
        $decode_from_charcode = "String.fromCharCode" ascii
        $script_end = "</script>" ascii
        
    condition:
        all of them
}