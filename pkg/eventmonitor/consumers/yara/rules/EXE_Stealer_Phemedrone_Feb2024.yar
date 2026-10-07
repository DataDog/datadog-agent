rule EXE_Stealer_Phemedrone_Feb2024 {
    meta:
        Description = "Detects Phemedrone Stealer malware samples"
        author = "Yashraj Solanki - Cyber Threat Intelligence Analyst at Bridewell"
        Reference = "https://bazaar.abuse.ch/browse/signature/PhemedroneStealer/"
        Hash = "6bccfdbe392cf2eef8a337fbb8af90a662773d8cd73cec1ac1e0f51686840215, 58b525579968cba0c68e8f7ae12e51e0b5542acc2c14a2e75fa6df44556e373f"
        date = "2024-02-10"
        yarahub_author_twitter = "@RustyNoob619"
        yarahub_reference_md5 = "22c2e9caea842dcd382cffa8fe73fff6"
        yarahub_uuid = "f48bf2ef-be21-4993-935f-a63dda092fc6"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        malpedia_family = "win.phemedrone_stealer"
    strings:
        $pheme1 = "Phemedrone"
        $pheme2 = "Phemedrone.Services"
        $pheme3 = "Phemedrone.Classes"
        $pheme4 = "Phemedrone.Protections"
        $pheme5 = "Phemedrone.Extensions"

        //Sandbox Detection 
        $vm1 = "AntiVM"
        $vm2 = "IsVM"
        $vm3 = "KillDebuggers"
        $vm4 = "debuggers"

        $pswd1 = "get_MasterPassword" 
        $pswd2 = "FormatPassword"
        $pswd3 = "ParsePasswords"
        $pswd4 = "DiscordList"
        $pswd5 = "PasswordList"
        $pswd6 = "masterPassword"
        $pswd7 = "password"
        $pswd8 = "masterPass"

        $crypto1 = "ParseColdWallets"
        $crypto2 = "CryptoWallets"
        $crypto3 = "ParseDatWallets"

        //Import Libraries found in strings but absent in PE Imports
        $unref1 = "kernel32.dll" 
        $unref2 = "rstrtmgr.dll"
        
       
    condition:
        any of ($pheme*)
        and 2 of ($vm*)
        and 4 of ($pswd*)
        and any of ($crypto*)
        and any of ($unref*)
       
 }