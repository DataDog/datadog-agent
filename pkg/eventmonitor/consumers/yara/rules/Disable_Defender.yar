rule Disable_Defender
{
	meta:
		author = "iam-py-test"
		description = "Detect files disabling or modifying Windows Defender, Windows Firewall, or Microsoft Smartscreen"
		false_positives = "Files modifying Defender for legitimate purposes, files containing registry keys related to Defender (i.e. diagnostic tools)"
		// Yarahub data
		yarahub_uuid = "1fcd3702-cf5b-47b4-919d-6372c5412151"
		date = "2022-11-19"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_reference_md5 = "799a7f1507e5e7328081a038987e9a6f"
		yarahub_author_twitter = "@iam_py_test"
	strings:
		// Windows Defender
		$defender_policies_reg_key = "\\SOFTWARE\\Policies\\Microsoft\\Windows Defender" ascii wide
		$defender_powershell_pupprotection_Force = "Set-MpPreference -Force -PUAProtection" ascii wide
		$defender_powershell_pupprotection = "Set-MpPreference -PUAProtection" ascii wide
		$defender_reg_key = "\\SOFTWARE\\Microsoft\\Windows Defender" ascii wide
		$defender_disable_autoexclusions_powershell_force = "Set-MpPreference -Force -DisableAutoExclusions" ascii wide
		$defender_disable_autoexclusions_powershell = "Set-MpPreference -DisableAutoExclusions" ascii wide
		$defender_disable_MAPS_reporting_force = "Set-MpPreference -Force -MAPSReporting" ascii wide
		$defender_disable_MAPS_reporting = "Set-MpPreference -MAPSReporting" ascii wide
		$defender_disable_submit_samples_force = "Set-MpPreference -Force -SubmitSamplesConsent" ascii wide
		$defender_disable_submit_samples = "Set-MpPreference -SubmitSamplesConsent" ascii wide
		$defender_disable_realtime_force = "Set-MpPreference -Force -DisableRealtimeMonitoring" ascii wide
		$defender_disable_realtime = "Set-MpPreference -DisableRealtimeMonitoring" ascii wide
		$defender_disable_IPS_force = "Set-MpPreference -Force -DisableIntrusionPreventionSystem" ascii wide
		$defender_disable_IPS = "Set-MpPreference -DisableIntrusionPreventionSystem" ascii wide
		$defender_wd_filter_driver = "%SystemRoot%\\System32\\drivers\\WdFilter.sys" ascii wide
		$defender_wdboot_driver = "%SystemRoot%\\System32\\drivers\\WdBoot.sys" ascii wide
		$defender_wdboot_driver_noenv = "C:\\Windows\\System32\\drivers\\WdBoot.sys" ascii wide
		$defender_net_stop_windefend = "net stop windefend" nocase ascii wide
		$defender_net_stop_SecurityHealthService = "net stop SecurityHealthService" nocase ascii wide
		$defender_powershell_exclusionpath = "Add-MpPreference -ExclusionPath" xor ascii wide
		$defender_powershell_exclusionpath_base64 = "Add-MpPreference -ExclusionPath" base64
		$defender_powershell_exclusionext = "Add-MpPreference -ExclusionExtension" ascii wide
		$defender_powershell_exclusionprocess = "Add-MpPreference -ExclusionProcess" ascii wide
		$defender_powershell_exclusionip = "Add-MpPreference -ExclusionIpAddress" ascii wide
		$defender_uilockdown = "Set-MpPreference -UILockdown" ascii wide
		$defender_uilockdown_force = "Set-MpPreference -Force -UILockdown" ascii wide
		$defender_securitycenter = "\\SOFTWARE\\Microsoft\\Windows Defender Security Center\\" ascii wide
		$defender_location = "C:\\Program Files (x86)\\Windows Defender\\" ascii wide
		$defender_clsid = "{6CED0DAA-4CDE-49C9-BA3A-AE163DC3D7AF}" nocase ascii wide
		$defender_powershell_checksigsscan = "Set-MpPreference -CheckForSignaturesBeforeRunningScan" ascii wide
		$defender_powershell_noscanarchive = "Set-MpPreference -DisableArchiveScanning" ascii wide
		$defender_powershell_nobmon = "Set-MpPreference -DisableBehaviorMonitoring" ascii wide
		$defender_powershell_noemail = "Set-MpPreference -DisableEmailScanning" ascii wide
		$defender_powershell_ioav = "Set-MpPreference -DisableIOAVProtection" ascii wide
		$defender_powershell_privacymode = "Set-MpPreference -DisablePrivacyMode" ascii wide
		$defender_powershell_sigschday = "Set-MpPreference -SignatureScheduleDay" ascii wide
		$defender_powershell_noremovescan = "Set-MpPreference -DisableRemovableDriveScanning" ascii wide
		$defender_powershell_changewindefend = "Set-Service -Name windefend -StartupType " nocase ascii wide
		$defender_powershell_changesecurityhealth = "Set-Service -Name securityhealthservice -StartupType " nocase ascii wide
		$defender_protocol_key = "HKEY_CLASSES_ROOT\\windowsdefender" nocase ascii wide
		$defender_powershell_controlledfolder_replace = "Set-MpPreference -ControlledFolderAccessAllowedApplications" nocase ascii wide
		$defender_powershell_controlledfolder_replace_force = "Set-MpPreference -Force -ControlledFolderAccessAllowedApplications" nocase ascii wide
		$defender_powershell_controlledfolder_add = "Add-MpPreference -ControlledFolderAccessAllowedApplications" nocase ascii wide
		$defender_powershell_controlledfolder_add_force = "Add-MpPreference -Force -ControlledFolderAccessAllowedApplications" nocase ascii wide
		$defender_powershell_DisableScanningMappedNetworkDrivesForFullScan = "Set-MpPreference -DisableScanningMappedNetworkDrivesForFullScan" nocase ascii wide
		$defender_powershell_malwareid = "Add-MpPreference -ThreatIDDefaultAction_Ids " nocase ascii wide
		$defender_Windows_Security_Health_key = "\\SOFTWARE\\Microsoft\\Windows Security Health" nocase ascii wide
		$defender_service = "\\SYSTEM\\ControlSet001\\Services\\EventLog\\System\\WinDefend" nocase ascii wide
		$defender_sc_stop = "sc stop WinDefend" nocase ascii wide
		$defender_sc_delete = "sc delete WinDefend" nocase ascii wide
		$defender_sc_disable = "sc config WinDefend start= disabled" nocase ascii wide
		$defender_powershell_uninstall_feature = "Uninstall-WindowsFeature -Name Windows-Defender" nocase ascii wide
		$defender_service_key_WdNisDrv = "\\System\\CurrentControlSet\\Services\\WdNisDrv" nocase ascii wide
		$defender_service_key_WdNisSvc = "\\System\\CurrentControlSet\\Services\\WdNisSvc" nocase ascii wide
		$defender_service_key_WdBoot = "\\System\\CurrentControlSet\\Services\\Wdboot" nocase ascii wide
		$defender_securityandmaint_key = "\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Security and Maintenance" ascii wide
		$defender_task_1 = "schtasks /Change /TN \"Microsoft\\Windows\\ExploitGuard\\ExploitGuard MDM policy Refresh\"" ascii wide
		$defender_task_2 = "schtasks /Change /TN \"Microsoft\\Windows\\Windows Defender\\Windows Defender Cache Maintenance\"" ascii wide
		$defender_task_3 = "schtasks /Change /TN \"Microsoft\\Windows\\Windows Defender\\Windows Defender Cleanup\"" ascii wide
		$defender_task_4 = "schtasks /Change /TN \"Microsoft\\Windows\\Windows Defender\\Windows Defender Scheduled Scan\"" ascii wide
		$defender_task_5 = "schtasks /Change /TN \"Microsoft\\Windows\\Windows Defender\\Windows Defender Verification\"" ascii wide nocase
		$defender_wmic = "WMIC /Namespace:\\\\root\\Microsoft\\Windows\\Defender" ascii wide nocase
		$defender_powershell_networkprotection = "Set-MpPreference -EnableNetworkProtection " ascii wide nocase
		$defender_restore_default = "\\MpCmdRun.exe -RestoreDefaults" ascii wide
		
		// Windows firewall
		$firewall_netsh_disable = "netsh advfirewall set allprofiles state off" ascii wide
		$firewall_reg_key = "\\SOFTWARE\\Policies\\Microsoft\\WindowsFirewall\\" ascii wide
		$firewall_sharedaccess_reg_key = "\\SYSTEM\\CurrentControlSet\\Services\\SharedAccess\\Parameters\\FirewallPolicy\\" ascii wide
		$firewall_allow = "netsh firewall add allowedprogram" nocase ascii wide
		$firewall_changelogsize = "netsh advfirewall set currentprofile logging maxfilesize" ascii wide nocase
		
		// Microsoft Windows Malicious Software Removal Tool
		$MRT_reg_key = "\\SOFTWARE\\Policies\\Microsoft\\MRT" ascii wide
		$MRT_reg_key_wow64 = "\\SOFTWARE\\WOW6432NODE\\POLICIES\\MICROSOFT\\MRT" ascii wide
		$MRT_del = "del C:\\Windows\\System32\\mrt.exe" nocase ascii wide
		
		// Edge
		$edge_phishing_filter = "\\SOFTWARE\\Policies\\Microsoft\\MicrosoftEdge\\PhishingFilter" ascii wide
		
		// Internet Explorer
		$ie_phishing_filter = "\\SOFTWARE\\Microsoft\\Internet Explorer\\PhishingFilter" ascii wide
		
		// key, value pairs - these may have false positives
		$k1 = "\\Software\\Microsoft\\Windows\\CurrentVersion\\Policies\\Explorer" ascii wide
		$k2 = "\\Software\\Microsoft\\Windows\\CurrentVersion\\Run" ascii wide
		$k3 = "\\SOFTWARE\\Policies\\Microsoft\\Windows\\System" ascii wide
		$k4 = "\\SOFTWARE\\MICROSOFT\\SECURITY CENTER" nocase ascii wide
		$k5 = "\\SYSTEM\\ControlSet001\\Services\\Sense" ascii wide
		
		$v1 = "HideSCAHealth" ascii wide
		$v2 = "SecurityHealth" ascii wide
		$v3 = "EnableSmartScreen" ascii wide
		$v4 = "FIREWALLDISABLENOTIFY" ascii wide nocase
		$v5 = "UPDATESDISABLENOTIFY" nocase ascii wide
		$v6 = "Start" nocase ascii wide

	condition:
		any of ($defender_*) or any of ($firewall_*) or any of ($MRT_*) or any of ($edge_*) or any of ($ie_*) or (1 of ($k*) and 1 of ($v*))
}