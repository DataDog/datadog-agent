rule Web_PHP_WP_FleetSync_EtherHiding
{
    meta:
        description = "WordPress fleet implant: fake helper plugin, persistence inventory, BNB-chain C2 resolver, remote file write/eval tasks."
        author = "Serhii Kocherhan"
        yarahub_twitter = "@skocherhan"
        date = "2026-09-23"
        malware_family = "WP_FleetSync"
        reference = "WordPress plugin backdoor with EtherHiding-style eth_call bootstrap"
        yarahub_uuid = "7ab2e39d-5389-4c18-bd2a-d6e720366872"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "00000000000000000000000000000000"

    strings:
        // Control / persistence markers (stable across builds)
        $m1 = "// ok_km" ascii
        $m2 = "// v0_3t" ascii
        $m3 = "// ea_plug" ascii
        $m4 = "// ea_mu" ascii
        $m5 = "// _ea_al" ascii
        $m6 = "// _ea_wc" ascii
        $m7 = "/* _ea_wc_s */" ascii
        $m8 = "/* _ea_wc_e */" ascii

        // Function / hook names
        $f1 = "function _ea_inv(" ascii
        $f2 = "function _ea_run_task(" ascii
        $f3 = "function _ea_sync(" ascii
        $f4 = "ea_fleet_sync" ascii
        $f5 = "'ea_fleet'" ascii

        // Task opcodes
        $t1 = "$_code==='wf'" ascii
        $t2 = "$_code==='ld'" ascii
        $t3 = "$_code==='su'" ascii
        $t4 = "$_code==='rp'" ascii
        $t5 = "$_code==='va'" ascii
        $t6 = "$_code==='fc'" ascii
        $t7 = "$_code==='ma'" ascii
        $t8 = "$_code==='fn'" ascii
        $t9 = "$_code==='wc'" ascii

        // Caps advertised to panel
        $c1 = "'server_sync'" ascii
        $c2 = "'visitor_js'" ascii
        $c3 = "'write_file'" ascii

        // Chain resolver shape (host/contract vary)
        $b1 = "eth_call" ascii
        $b2 = "jsonrpc" ascii
        $b3 = "'method'=>'eth_call'" ascii
        $b4 = "\"verify_peer\"=>false" ascii
        $b5 = "bnbchain.org" ascii
        $b6 = "binance.org" ascii

        // HTTP C2 shape (FQDN varies)
        $h1 = "X-HSS-Auth" ascii
        $h2 = "X-Site-Id" ascii
        $h3 = "/panel/api/v1/metrics/collect" ascii
        $h4 = "\"sslverify\"=>false" ascii
        $h5 = "wp_remote_post" ascii

        // Inventory targets
        $i1 = "mu_autologin" ascii
        $i2 = "functions_autologin" ascii
        $i3 = "wpconfig_inject" ascii
        $i4 = "themes_with_marker" ascii

        $eval = "eval($_p['code'])" ascii

    condition:
        filesize < 200KB
        and (
            3 of ($m*)
            or 3 of ($f*)
            or (2 of ($f*) and 4 of ($t*))
        )
        and (
            $eval
            or 2 of ($h*)
            or 2 of ($b*)
        )
        and 2 of ($i*)
        and any of ($c*)
}