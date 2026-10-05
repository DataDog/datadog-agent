rule EDR_Killer_EDR_Freeze_Tool {
    meta:
        description = "Detects EDR-Freeze tool in memory - EDR/AV freezing malware"
        author = "Valton Tahiri (cybee.ai)"
        date = "2025-10-09"
        reference = "https://www.linkedin.com/in/valton-tahiri/"
        severity = "critical"
        category = "edr_killer"
        malware_family = "EDR-Freeze"
        hash_sample = "ff6f1a93c2e0b46d9a3e18c75d"
        yarahub_reference_md5 = "2c8fbd0f7fd0ed8ebcacb087c8faa6f3"
        yarahub_uuid = "e7b8c5f5-7d3f-4e02-9b3c-2f3f0a2a3c9d"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        $tool_banner = "EDR-Freeze: Tool that freezes EDR/Antivirus" ascii wide
        $pdb_path = "D:\\Projects\\PPL\\EDR-Freeze\\x64\\Release\\EDR-Freeze.pdb" ascii wide
        $usage1 = "EDR-Freeze.exe <TargetPID> <SleepTime>" ascii wide
        $usage2 = "EDR-Freeze.exe 1234 10000" ascii wide
        $op_freeze = "Freeze the target for 10000 milliseconds" ascii wide
        $op_ppl_create = "Successfully created PPL process with PID:" ascii wide
        $op_target_paused = "Target paused. PID:" ascii wide
        $op_wer_paused = "WER paused. PID:" ascii wide
        $op_kill_wer_success = "Kill WER successfully. PID:" ascii wide
        $op_kill_wer_failed = "Kill WER failed:" ascii wide
        $msg_suspended = "Process suspended successfully." ascii wide
        $msg_terminated = "Process terminated successfully." ascii wide
        $prot_level1 = "PROTECTION_LEVEL_ANTIMALWARE_LIGHT" ascii wide
        $prot_level2 = "PROTECTION_LEVEL_WINTCB_LIGHT" ascii wide
        $prot_level3 = "PROTECTION_LEVEL_WINDOWS_LIGHT" ascii wide
        $prot_level4 = "PROTECTION_LEVEL_LSA_LIGHT" ascii wide
        $prot_level5 = "PROTECTION_LEVEL_WINDOWS" ascii wide
        $prot_display = "Protection Level:" ascii wide
        $err_ppl_create = "Failed to create PPL process." ascii wide
        $err_main_thread = "Failed to find main thread for PID" ascii wide
        $err_update_proc = "UpdateProcThreadAttribute failed:" ascii wide
        $err_init_proc = "InitializeProcThreadAttributeList failed:" ascii wide
        $priv_debug = "SeDebugPrivilege enabled successfully." ascii wide
        $priv_failed = "Failed to enable debug privilege." ascii wide
        $api_suspend = "OpenProcess: PROCESS_SUSPEND_RESUME failed:" ascii wide
        $api_ntsuspend = "NtSuspendProcess failed. Error code:" ascii wide
        $target_wer = "C:\\Windows\\System32\\WerFaultSecure.exe" ascii wide
        $author_twitter = "Two Seven One Three: https://x.com/TwoSevenOneT" ascii wide
        $ntapi_suspend = "NtSuspendProcess" ascii wide
        $ntapi_query = "NtQuerySystemInformation" ascii wide

    condition:
        uint16(0) == 0x5A4D and
        (
            ($tool_banner or $pdb_path) or
            (3 of ($op_*)) or
            (2 of ($prot_level*) and 2 of ($op_*)) or
            (any of ($usage*) and 2 of ($op_*)) or
            ($author_twitter and 2 of ($op_*)) or
            (any of ($priv_*) and $target_wer and 2 of ($op_*)) or
            (2 of ($err_*) and any of ($ntapi_*) and any of ($prot_*)) or
            (any of ($msg_*) and 2 of ($op_*) and any of ($ntapi_*)) or
            (any of ($api_*) and 2 of ($op_*) and any of ($prot_level*))
        )
}
