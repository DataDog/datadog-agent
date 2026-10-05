
rule EXE_ICS_IronGate_April2024 {
    meta:
        Description = "Detects Iron Gate ICS malware targeting simulation environment which appears to be a PoC"
       author = "Yashraj Solanki - Cyber Threat Intelligence Analyst at Bridewell"
        Reference = "https://www.mandiant.com/resources/blog/irongate-ics-malware"
        File_Hash = "0539af1a0cc7f231af8f135920a990321529479f6534c3b64e571d490e1514c3"
        Info = "this YARA detects both the Dropper (EXE) and the final payload (DLL) for the Iron Gate malware "
        date = "2024-04-07"
        yarahub_author_twitter = "@RustyNoob619"
        yarahub_reference_md5 = "874f7bcab71f4745ea6cda2e2fb5a78c"
        yarahub_uuid = "636f11f2-9c55-42a8-a4ef-b8a5d2cc8b18"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        $step1 = "Step7ConMgr.dll" wide fullword // malware searches for the legit DLL Step7ProSim and then renames it to this one
        $step2 = "Step7ProSim.COM" wide fullword 
        $step3 = "Step7ProSimProxy" wide fullword
        $step4 = "Step7ProSim.dll" wide fullword // when certain conditions are met, the malware will drop this malicious DLL

        $cmd1 = "WriteInputPoint" wide fullword
        $cmd2 = "ReadDataBlockValue" wide fullword
        $cmd3 = "WriteDataBlockValue" wide fullword
        $cmd4 = "ReadOutputPoint" wide fullword
        $cmd5 = "SetState" wide fullword
        

        $s71 = "Step7ProSim.dll" fullword
        $s72 = "IStep7ProSim" fullword
        $s73 = "Step7ProSim.Interfaces" fullword
        $s74 = "Step7ProSim" fullword

        $plcsim1 = "RunPlcSim" fullword
        $plcsim2 = "StartPLCSim" wide fullword

        $module1 = "ReadDataBlockSingle" fullword
        $module2 = "ReadDataBlockValue" fullword
        $module3 = "ReadOutputPoint" fullword
        $module4 = "WriteDataBlockSingle" fullword
        $module5 = "WriteDataBlockValue" fullword
        $module7 = "WriteInputPoint" fullword
        
        $playback1 = "waitBeforeRecordingTimeInMilliSeconds" fullword
        $playback2 = "waitBeforePlayingRecordsTimeInMilliSeconds" fullword
        $playback3 = "payloadExecutionTimeInMilliSeconds" fullword
        $playback4 = "waitBeforePlayingRecordsTimer" fullword
        $playback5 = "waitBeforeRecordingTimer" fullword
        $playback6 = "payloadExecutionTimer" fullword

        $dotnet1 = ".NETFramework,Version=v4.0"
        $dotnet2 = ".NET Framework 4"
        
    condition:
        uint16(0) == 0x5a4d
        and any of ($dotnet*)
        and any of ($step*)
        and 2 of ($cmd*)
        and any of ($plcsim*)
        and any of ($s7*)
        and 2 of ($module*)
        and 2 of ($playback*)
        and filesize < 1MB
        
 }









