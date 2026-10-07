rule Seedhook_Wallet_ExfilPage
{
    meta:
        yarahub_uuid = "34bba3ad-1ba4-426d-9593-726150af750a"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "b137df2d7938e9a9afa10f24bc32b9bc"
        description = "Seedhook/MacSync fake wallet-recovery page (HTML/JS, standalone or inside app.asar): 64-hex campaign token then targetUrl = https://<host>/modules/wallets POSTed as JSON, plus the page's own DOM wiring (several listeners and element lookups) so that notes or tools merely quoting the handler do not match"
        actor = "Seedhook (MacSync Stealer cluster)"
        reference_sha256 = "6fa7543b68fe5b931a8fa7f4b1be3990b6e9ceaa90ec6c14d9f15cdaab98023e"
        date = "2026-09-19"
        tlp = "CLEAR"

    strings:
        // the exfil handler template (a quotation usually stops here)
        $exfil    = /[0-9a-f]{64}'\s*;\s*(const|var)\s+targetUrl\s*=\s*'https:\/\/[a-z0-9.-]{4,80}\/modules\/wallets'/ ascii
        $fetch    = "fetch(targetUrl, {" ascii
        $post     = /method:\s*'POST'/ ascii
        $body     = /body:\s*JSON\.stringify\(\{/ ascii
        // page mechanics a quotation does not carry: the form wiring around the handler
        $ev       = "addEventListener(" ascii
        $dom1     = "document.getElementById(" ascii
        $dom2     = "querySelectorAll(" ascii

    condition:
        $exfil and $fetch and $post and $body and
        #ev >= 3 and (#dom1 + #dom2) >= 2
}
