rule GreenBloodRansomware_v22t : Ransomware
{
  meta:
    description = "Detects GreenBlood ransomware family"
    author = "Valton Tahiri"
    reference = "https://www.linkedin.com/in/valton-tahiri/"
    date = "2026-02-12"

    /* --- YARAify / YARAhub required fields --- */
    yarahub_uuid = "2d2f6f51-4f6a-4d21-9d2e-9b9c1f5e7a6b"
    yarahub_license = "CC0 1.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"
    yarahub_reference_md5 = "122ba7161d07efcb1b14d30054901ac9"

    /* extra context */
    category = "ransomware"
    malware_family = "GreenBlood"
    severity = "critical"
    tlp = "TLP:WHITE"

  strings:
    /* unique identifiers */
    $email   = "thegreenblood@proton.me" ascii wide nocase
    $banner  = "TH3 GR33N BL00D GR0UP" ascii wide
    $encpp   = "enc++" ascii wide
    $note1   = "ALL YOUR FILES HAVE BEEN ENCRYPTED!" ascii wide
    $note2   = "DO NOT ATTEMPT TO DECRYPT FILES YOURSELF!" ascii wide
    $subid   = "DAF-SN-" ascii wide
    $cleanup = "cleanup_greenblood.bat" ascii wide nocase

    /* destructive behavior (only counted in combination) */
    $vss     = "vssadmin delete shadows /all /quiet" ascii wide nocase
    $defkey  = "Windows Defender\\Real-Time Protection" ascii wide
    $rtm     = "DisableRealtimeMonitoring" ascii wide

  condition:
    ($email or $banner) and
    (2 of ($encpp,$note1,$note2,$subid,$cleanup,$vss,$rtm,$defkey))
}
