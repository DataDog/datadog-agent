rule tarrarat_license_server
{
    meta:
        yarahub_uuid = "739584d2-a741-4ac1-a09f-bb8ce6e897ab"
        yarahub_license = "CC0 1.0"
        yarahub_author_twitter = "@Lenny_3BO"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "a44fd5e62ca073c5b45d782cdb4624ee"
        author      = "Lenny-3BO"
        description = "TarraRAT Node.js license server -- Telegram HWID bind + /api/validate route"
        family      = "win.tarrarat"
        version     = "1.0"
        date        = "2026-04-30"
        reference   = "hunts/omegatech-as202412-tun1-sweep"

    strings:
        // Express API route unique to this server
        $api_route      = "/api/validate" ascii

        // Telegram bot library import
        $tg_bot         = "node-telegram-bot-api" ascii

        // HWID workflow comment in Spanish (operator language)
        $hwid_reset     = "Reset HWID" ascii

        // License table column signature
        $hwid_col       = "hwid TEXT DEFAULT NULL" ascii

        // Operator bat launcher (Spanish -- INICIAR/INSTALAR)
        $bat_iniciar    = "INICIAR_SERVIDOR" ascii

    condition:
        filesize < 5MB and
        $api_route and
        2 of ($tg_bot, $hwid_reset, $hwid_col, $bat_iniciar)
}
