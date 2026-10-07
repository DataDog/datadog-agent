rule Embedded_RTF_File
{
    meta:
        author = "Nicholas Dhaeyer - @DhaeyerWolf"
        date_created = "2023-07-18"
        date_last_modified = "2023-07-18"
        description = "Related to CVE-2023-36884. Hunts for any zip-like archive (eg. office documents) that have an embedded .rtf file, based on the '.rtf' extension of the file."
		yarahub_uuid = "800682b8-e810-49d2-91b3-dfaafb61637f"
		date = "2023-07-18"
		yarahub_license = "CC BY-SA 4.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_reference_md5 = "b6ad6198e155921dc11c855c03d8c264"

    strings:
		$header = { 50 4B 03 04 } //beginning of a archive file
		$header1 = { D0 CF 11 E0 A1 B1 1A E1 } //Older formats of office files
	
        $rtf =  { 2E 72 74 66 } //.rtf
		
		$str1 = "Microsoft Office Word" //doc
		$str2 = "MSWordDoc" //doc
		$str3 = "Word.Document.8" //doc
		$str4 = "Microsoft Office PowerPoint" //ppt
		$str5 = "Microsoft Excel" //xls
		$str6 = "Excel.Sheet.8" //xls
		$str7 = "document.xml" //docx
		$str8 = "presentation.xml" //pptx
		$str9 = "workbook.xml" //xlsx
		$str10 = "workbook.bin" //xlsb
		$str11 = "<?mso-application progid=\"Word.Document\"?>" //word_xml
		$str12 = "<?mso-application progid=\"PowerPoint.Show\"?>" //ppt_xml
		$str13 = "<?mso-application progid=\"Excel.Sheet\"?>" //Excel_xml
		
    condition:
        ($header at 0 or $header1 at 0)
		and (#rtf > 1)
		and 1 of ($str*)
}