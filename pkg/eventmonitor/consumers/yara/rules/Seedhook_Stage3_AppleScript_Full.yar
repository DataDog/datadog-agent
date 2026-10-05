rule Seedhook_Stage3_AppleScript_Full
{
    meta:
        yarahub_uuid = "67347a7c-8aab-4c3c-a864-2ecbbe2ac22b"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "bef4d496763a034c1e0af22a65c6f212"
        description = "Seedhook/MacSync stage-3 AppleScript 'full' build: Ledger/Trezor app.asar swap block (LEDGER* variables) and/or com.apple.<8hex>.hcpi LaunchAgent + /loader/agent/ backdoor install block"
        actor = "Seedhook (MacSync Stealer cluster)"
        reference_sha256 = "b647c8189ad6305e940bd904e496bde966cdcba333e9044a1567cf98d707d959"
        date = "2026-09-19"
        tlp = "CLEAR"

    strings:
        $ctx  = "do shell script" ascii
        // trojanized wallet swap block (repeated once per wallet app)
        $wal1 = "LEDGERDMGPATH" ascii
        $wal2 = "LEDGERTMPDEST" ascii
        $wal3 = "set ledger_installed to" ascii
        $wal4 = "/Contents/Resources/app.asar" ascii
        // loader_agent backdoor install + LaunchAgent persistence block
        $per1 = /com\.apple\.[0-9a-f]{8}\.hcpi/ ascii
        $per2 = ".loader_install.log" ascii
        $per3 = "/loader/agent/" ascii
        $per4 = "somesystemPlist" ascii

    condition:
        filesize < 1MB and $ctx and (3 of ($wal*) or 3 of ($per*))
}
