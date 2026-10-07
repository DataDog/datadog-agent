rule KarstoRAT_Build4
{
    meta:
        id = "wkmOHSBtmAzawl95rZjcmX"
        fingerprint = "f0a3ed8d4c25d8360886c2524e66fa56fa4f86d1a2c1915c3017b8d2353a4a2f"
        version = "1.0"
        date = "2026-03-10"
        modified = "2026-03-10"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://GITHUB.COM/KIRKDERP/YARA"
        author = "derp.ca"
        description = "KarstoRAT Build 4 variant - anti-analysis, PLAY_SOUND, upload endpoint bug"
        category = "MALWARE"
        malware = "KARSTORAT"
        malware_type = "RAT"
        mitre_att = "T1497.001"
        reference = "https://github.com/kirkderp/yara"
        triage_score = 10
        triage_description = "KarstoRAT Build 4 with anti-analysis and PLAY_SOUND command detected."
        yarahub_uuid = "c37893ae-659a-4652-af9d-19eab5553d4b"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "a5bef919eb260af5bb8eba243ed4fd75"

    strings:
        $pdb = "\\Users\\hibby\\Desktop\\Project1\\" ascii
        $ua = "SecurityNotifier" ascii wide

        // Anti-analysis process blocklist (Build 4 only)
        $av_x64dbg = "x64dbg.exe" ascii wide
        $av_wireshark = "wireshark.exe" ascii wide
        $av_vmware = "vmware.exe" ascii wide
        $av_vmtoolsd = "vmtoolsd.exe" ascii wide
        $av_vmsrvc = "vmsrvc.exe" ascii wide
        $av_vmusrvc = "vmusrvc.exe" ascii wide
        $av_vboxtray = "vboxtray.exe" ascii wide
        $av_vboxservice = "vboxservice.exe" ascii wide
        $av_df5serv = "df5serv.exe" ascii wide

        // PLAY_SOUND command (Build 4 only)
        $cmd_playsound = "PLAY_SOUND:" ascii wide
        $alarm_file = "alarm.wav" ascii wide

        // Upload endpoint bug - & instead of ? (Build 4 regression)
        $upload_bug = "/client-upload&filename=" ascii

        // Build 4 simplified response format (no "Action: [" prefix on some)
        $resp_simplified1 = "Added to Startup" ascii
        $resp_simplified2 = "Task Created" ascii
        $resp_simplified3 = "Copied to Folder" ascii

    condition:
        uint16(0) == 0x5A4D
        and filesize > 50KB and filesize < 500KB
        and ($pdb or $ua)
        and (
            // Anti-analysis: 5+ of the 9 blocklist names
            (5 of ($av_*))
            or
            // PLAY_SOUND feature
            ($cmd_playsound and $alarm_file)
            or
            // Upload endpoint bug (unique to Build 4)
            $upload_bug
            or
            // Build 4 simplified responses + anti-analysis combo
            (3 of ($resp_simplified*) and 3 of ($av_*))
        )
}
