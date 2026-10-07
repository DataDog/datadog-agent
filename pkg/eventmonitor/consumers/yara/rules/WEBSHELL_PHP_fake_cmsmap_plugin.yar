rule WEBSHELL_PHP_fake_cmsmap_plugin
{
    meta:
        description     = "Fake WordPress plugin impersonating CMSmap, uploaded through wp-admin's plugin installer. Two payloads share the header: a loader that includes a webshell hidden in log.db, and a dropper that inserts an administrator named adminlin. Strings are digit-substituted per build (CMSsmap, Sh21ll, C1MSmap1, GPLv5) but the disclaimer sentence is carried verbatim. The real CMSmap is a Python tool, so this header in a PHP file is always a forgery."
        author          = "Efrain Gutierrez"
        date            = "2026-09-17"

        yarahub_uuid              = "57bd839a-f4f6-42cf-9a40-ef17b92969ad"
        yarahub_reference_md5     = "6f2394f0f643a1395159eec20976dbc6"
        yarahub_license           = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"

        loader_sha256   = "10cf2e3f52b5daee5ee47fb639a17003f019a3d8f9d0c0fc8fdfbcc71ff88453"
        dropper_sha256  = "4e2a6a49b22ebee5fc155eb145e02f5c13b0f8355680fbcbbeae9ac554110b7c"
        note            = "Execution is a GET on the plugin directory: index.php is served as the directory index, so requesting the folder runs the payload."

    strings:
        // Identical in every build seen, whatever else is mutated.
        $disclaimer = "Usage of CMSmap for attacking targets without prior mutual consent is illegal" ascii

        // Loader: silences errors, survives a dropped connection, pulls in the
        // shell from a file named to look like data.
        $load_abort = "ignore_user_abort(true)" ascii
        $load_inc   = "include('log.db')"       ascii

        // Dropper: bootstraps WordPress for $wpdb, then writes the account.
        $drop_boot  = "wp-blog-header.php"                     ascii
        $drop_user  = "adminlin"                               ascii
        $drop_pass  = "57a48cf5883989417e6c0583c87ceb40"       ascii
        $drop_mail  = "admin@admin.com"                        ascii

    condition:
        filesize < 50KB
        and uint32be(0) == 0x3C3F7068      // "<?ph"
        and (
            $disclaimer
            or all of ($load_*)
            or 2 of ($drop_*)
        )
}
