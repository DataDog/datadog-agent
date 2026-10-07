rule EXE_Ransomware_Mimic
{
    meta:
        author = "Yashraj Solanki - Cyber Threat Intelligence Analyst at Bridewell"
        description = "Detects Mimic ransomware samples based on the strings matched"
        source = "https://www.securonix.com/blog/securonix-threat-research-security-advisory-new-returgence-attack-campaign-turkish-hackers-target-mssql-servers-to-deliver-domain-wide-mimic-ransomware/"
        hash = "d6cd0080d401be8a91a55b006795701680073df8cd7a0b5bc54e314370549dc4"
        date = "2024-01-17"
        yarahub_author_twitter = "@RustyNoob619"
        yarahub_reference_md5 = "98e9fd3bcd9e94f5a8b2566c9dcf97d2"
        yarahub_uuid = "ef2757d5-267a-4d4e-92c3-9bbfb56b8e76"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        malpedia_family = "win.mimic"
    strings:
        $str1 = "MIMIC_LOG.txt" wide 
        $str2 = "mimicfile" wide
        $str3 = "Mimic" wide
        $crpt1 = "crypto\\evp\\evp_key.c" 
        $crpt2 = "crypto\\x509v3\\v3_conf.c" 
        $crpt3 = "EVP_EncryptUpdate" 
        $crpt4 = "EVP_EncryptFinal_ex" 
        $cmd1 = "Delete Shadow Copies" wide
        $cmd2 = "Loading hidden partitions" wide  
        $cmd3 = "SELECT * FROM Win32_ShadowCopy" wide  
        $cmd4 = "Attempt to unlock file" wide  
        $cmd5 = "SetPrivilege" wide  
        $cmd6 = "ClearBackup" wide  
        $cmd7 = "ConsentPromptBehaviorAdmin" wide 
        
    condition:
        uint16(0) == 0x5A4D
        and all of ($str*) 
        and 2 of ($crpt*) 
        and 4 of ($cmd*)
}