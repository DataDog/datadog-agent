rule Corporate_JS_Obfuscator_Fingerprint_Carrier {
    meta:
        description = "Hardened regex detection for JavaScript loaders utilizing environment-gating hooks"
        author      = "Sean Taylor | Amethyst Systems"
        date        = "2026-08-24"
        platform    = "Windows / Script Host"
        target_type = "JS, JSE"
        reference   = "ANY.RUN XWorm/XLoader Triage"
        yarahub_author = "YourHandle"
        yarahub_reference = "https://any.run"
        yarahub_uuid = "0bc53a02-c795-4e9c-88ae-c69fb6bd812e"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "fbc8221bf9b6198c6111b981a78956e3"

    strings:
        $obfuscator_sig = /@metadata\s+version=[0-9\.]+\s+build=[0-9]+/ ascii
        $hook_engine  = /ScriptEngine\s*\(/ ascii
        $hook_wscript = /typeof\s+WScript/ ascii
        $hook_activex = /typeof\s+ActiveXObject/ ascii
        $math_bitwise = "& 255" ascii
        $math_catch   = /catch\s*\(/ ascii

    condition:
        (((2 of ($hook_*)) and (1 of ($math_*))) or $obfuscator_sig) 
        and filesize < 500KB
}
