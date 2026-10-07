/* Realizado por Nerio Rodriguez */
rule xlsb_adj 
{
    meta:
        description = "Regla para correo malicioso (adjunto)"
        author = "Nerio Rodriguez"
        date = "2024-04-15"
	yarahub_uuid = "d8e0bae3-306f-4e95-bb63-49021ccaf56c"
        yarahub_reference_md5 = "c2293ce082da26ff050854765bcd0870"
	yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
    strings:
        $s1 = "Payment Remittance Advice_000000202213.xlsb" wide ascii
    condition:
        all of them
}