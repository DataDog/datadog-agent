rule StealcConfig {
    meta:
        author = "NDA0E"
        yarahub_author_twitter = "@NDA0E"
        date = "2024-10-20"
        description = "Detects Stealc Config"
        yarahub_uuid = "d41f4122-23e5-42bb-b81f-7545dba4de1d"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "72c5ce737eff339731b4b5de012d5478"
        malpedia_family = "win.stealc"
    
    strings:
        $timeoutc = "/c timeout /t 10 & del /f /q \"" ascii
        $cryptkn = "SELECT service, encrypted_token FROM token_service" ascii
        $phpcfg = /\/[a-zA-Z0-9]{16}\.php/ ascii
        
    condition:
        uint16(0) == 0x5a4d and 
        all of them
}