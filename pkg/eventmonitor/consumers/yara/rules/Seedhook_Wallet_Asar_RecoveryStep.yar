rule Seedhook_Wallet_Asar_RecoveryStep
{
    meta:
        yarahub_uuid = "b3d2adf1-9df4-46b1-9f81-483785a80045"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "b137df2d7938e9a9afa10f24bc32b9bc"
        description = "Seedhook/MacSync trojanized Ledger/Trezor Electron app.asar: injected recovery-step page loaded by the main process, seed phrase POSTed to <host>/modules/wallets with a 64-hex token"
        actor = "Seedhook (MacSync Stealer cluster)"
        reference_sha256 = "6fa7543b68fe5b931a8fa7f4b1be3990b6e9ceaa90ec6c14d9f15cdaab98023e"
        date = "2026-09-19"
        tlp = "CLEAR"

    strings:
        // asar header JSON starts at offset 16
        $hdr      = "{\"files\":{" ascii
        // injected page name appears in the asar index and in the main-process loader call
        $page     = "recovery-step-1.html" ascii
        // main process swaps the window to the fake page after a delay
        $load1    = /setTimeout\(\s*\(\)\s*=>\s*\{\s*e\.load(URL|File)\([^\n]{0,120}recovery-step-1\.html/ ascii
        // campaign token followed by the exfil URL template
        $exfil    = /[0-9a-f]{64}'\s*;\s*(const|var)\s+targetUrl\s*=\s*'https:\/\/[a-z0-9.-]{4,80}\/modules\/wallets'/ ascii
        $fetch    = "fetch(targetUrl, {" ascii

    condition:
        uint32(0) == 4 and $hdr at 16 and filesize > 1MB and
        #page >= 2 and $exfil and $fetch and $load1
}
