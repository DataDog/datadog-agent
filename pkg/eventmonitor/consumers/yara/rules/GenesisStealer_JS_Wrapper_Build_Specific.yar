rule GenesisStealer_JS_Wrapper_Build_Specific
{
    meta:
        author                      = "n3r"
        description                 = "GenesisStealer outer JS wrapper (stage_1). Detects the campaign-specific PBKDF2-HMAC-SHA512 + AES-256-CBC decryption chain (password / IV / salt baked into the obfuscated JS that decrypts the inner stealer)."
        date                        = "2026-05-28"
        sha256                      = "fa83180ee18c87e91ab920252e77692e7849b03d8220ace614bd4620bc559bb8"
        family                      = "GenesisStealer"
        scope                       = "outer obfuscated JS wrapper (stage_1) - build-specific to this campaign (PBKDF2 password / IV / salt)"
        confidence                  = "HIGH"
        yarahub_uuid                = "0d4dc0a3-193a-4b14-945d-8ab02d240515"
        yarahub_license             = "CC BY 4.0"
        yarahub_reference_md5       = "d1634b0162d5784d989cfa421f5b3db2"
        yarahub_reference_link      = "https://bazaar.abuse.ch/sample/fa83180ee18c87e91ab920252e77692e7849b03d8220ace614bd4620bc559bb8/"
        yarahub_rule_matching_tlp   = "TLP:WHITE"
        yarahub_rule_sharing_tlp    = "TLP:WHITE"

    strings:
        $pbkdf2_pass = "8X9+CehDwyGExpfoGWcausHkyQJp13p+" ascii wide
        $iv_hex      = "12bd438e2e1a651e236548e32152c0f5" ascii wide
        $salt_hex    = "f20e04286bbff3d4a73df7a44f843c3c" ascii wide
        $rot_helper  = "_88259c15" ascii wide

    condition:
        2 of them
}
