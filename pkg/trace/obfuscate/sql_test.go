// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	"github.com/stretchr/testify/assert"
)

type sqlTestCase struct {
	query    string
	expected string
}

type sqlTokenizerTestCase struct {
	str          string
	expected     string
	expectedKind TokenKind
}

func SQLSpan(query string) *pb.Span {
	return &pb.Span{
		Resource: query,
		Type:     "sql",
		Meta: map[string]string{
			"sql.query": query,
		},
	}
}

func TestSQLResourceQuery(t *testing.T) {
	assert := assert.New(t)
	span := &pb.Span{
		Resource: "SELECT * FROM users WHERE id = 42",
		Type:     "sql",
		Meta: map[string]string{
			"sql.query": "SELECT * FROM users WHERE id = 42",
		},
	}

	NewObfuscator(nil).Obfuscate(span)
	assert.Equal("SELECT * FROM users WHERE id = ?", span.Resource)
	assert.Equal("SELECT * FROM users WHERE id = 42", span.Meta["sql.query"])
}

func TestSQLResourceWithoutQuery(t *testing.T) {
	assert := assert.New(t)
	span := &pb.Span{
		Resource: "SELECT * FROM users WHERE id = 42",
		Type:     "sql",
	}

	NewObfuscator(nil).Obfuscate(span)
	assert.Equal("SELECT * FROM users WHERE id = ?", span.Resource)
	assert.Equal("SELECT * FROM users WHERE id = ?", span.Meta["sql.query"])
}

func TestKeepSQLAlias(t *testing.T) {
	q := `SELECT username AS person FROM users WHERE id=4`

	t.Run("off", func(t *testing.T) {
		oq, err := NewObfuscator(nil).ObfuscateSQLString(q)
		assert.NoError(t, err)
		assert.Equal(t, "SELECT username FROM users WHERE id = ?", oq.Query)
	})

	t.Run("on", func(t *testing.T) {
		defer testutil.WithFeatures("keep_sql_alias")()
		oq, err := NewObfuscator(nil).ObfuscateSQLString(q)
		assert.NoError(t, err)
		assert.Equal(t, "SELECT username AS person FROM users WHERE id = ?", oq.Query)
	})
}

func TestDollarQuotedFunc(t *testing.T) {
	q := `SELECT $func$INSERT INTO table VALUES ('a', 1, 2)$func$ FROM users`

	t.Run("off", func(t *testing.T) {
		oq, err := NewObfuscator(nil).ObfuscateSQLString(q)
		assert.NoError(t, err)
		assert.Equal(t, "SELECT ? FROM users", oq.Query)
	})

	t.Run("on", func(t *testing.T) {
		defer testutil.WithFeatures("dollar_quoted_func")()
		oq, err := NewObfuscator(nil).ObfuscateSQLString(q)
		assert.NoError(t, err)
		assert.Equal(t, `SELECT $func$INSERT INTO table VALUES ( ? )$func$ FROM users`, oq.Query)
	})

	t.Run("AS", func(t *testing.T) {
		defer testutil.WithFeatures("keep_sql_alias,dollar_quoted_func")()
		oq, err := NewObfuscator(nil).ObfuscateSQLString(`CREATE OR REPLACE FUNCTION pg_temp.sequelize_upsert(OUT created boolean, OUT primary_key text) AS $func$ BEGIN INSERT INTO "school" ("id","organization_id","name","created_at","updated_at") VALUES ('dc4e9444-d7c9-40a9-bcef-68e4cc594e61','ec647f56-f27a-49a1-84af-021ad0a19f21','Test','2021-03-31 16:30:43.915 +00:00','2021-03-31 16:30:43.915 +00:00'); created := true; EXCEPTION WHEN unique_violation THEN UPDATE "school" SET "id"='dc4e9444-d7c9-40a9-bcef-68e4cc594e61',"organization_id"='ec647f56-f27a-49a1-84af-021ad0a19f21',"name"='Test',"updated_at"='2021-03-31 16:30:43.915 +00:00' WHERE ("id" = 'dc4e9444-d7c9-40a9-bcef-68e4cc594e61'); created := false; END; $func$ LANGUAGE plpgsql; SELECT * FROM pg_temp.sequelize_upsert();`)
		assert.NoError(t, err)
		assert.Equal(t, `CREATE OR REPLACE FUNCTION pg_temp.sequelize_upsert ( OUT created boolean, OUT primary_key text ) AS $func$BEGIN INSERT INTO school ( id, organization_id, name, created_at, updated_at ) VALUES ( ? ) created := ? EXCEPTION WHEN unique_violation THEN UPDATE school SET id = ? organization_id = ? name = ? updated_at = ? WHERE ( id = ? ) created := ? END$func$ LANGUAGE plpgsql SELECT * FROM pg_temp.sequelize_upsert ( )`, oq.Query)
	})
}

func TestScanDollarQuotedString(t *testing.T) {
	for _, tt := range []struct {
		in  string
		out string
		err bool
	}{
		{`$tag$abc$tag$`, `abc`, false},
		{`$func$abc$func$`, `abc`, false},
		{`$tag$textwith\n\rnewlinesand\r\\\$tag$`, `textwith\n\rnewlinesand\r\\\`, false},
		{`$tag$ab$tactac$tx$tag$`, `ab$tactac$tx`, false},
		{`$$abc$$`, `abc`, false},
		{`$$abc`, `abc`, true},
		{`$$abc$`, `abc`, true},
	} {
		t.Run("", func(t *testing.T) {
			tok := NewSQLTokenizer(tt.in, false)
			kind, str := tok.Scan()
			if tt.err {
				if kind != LexError {
					t.Fatalf("Expected error, got %s", kind)
				}
				return
			}
			assert.Equal(t, string(str), tt.out)
			assert.Equal(t, DollarQuotedString, kind)
		})
	}

	t.Run("dollar_quoted_func", func(t *testing.T) {
		t.Run("off", func(t *testing.T) {
			tok := NewSQLTokenizer("$func$abc$func$", false)
			kind, _ := tok.Scan()
			assert.Equal(t, DollarQuotedString, kind)
		})

		t.Run("on", func(t *testing.T) {
			defer testutil.WithFeatures("dollar_quoted_func")()
			tok := NewSQLTokenizer("$func$abc$func$", false)
			kind, _ := tok.Scan()
			assert.Equal(t, DollarQuotedFunc, kind)
		})
	})
}

func TestSQLResourceWithError(t *testing.T) {
	assert := assert.New(t)
	testCases := []struct {
		span pb.Span
	}{
		{
			pb.Span{
				Resource: "SELECT * FROM users WHERE id = '' AND '",
				Type:     "sql",
			},
		},
		{
			pb.Span{
				Resource: "INSERT INTO pages (id, name) VALUES (%(id0)s, %(name0)s), (%(id1)s, %(name1",
				Type:     "sql",
			},
		},
		{
			pb.Span{
				Resource: "INSERT INTO pages (id, name) VALUES (%(id0)s, %(name0)s), (%(id1)s, %(name1)",
				Type:     "sql",
			},
		},
	}

	for _, tc := range testCases {
		NewObfuscator(nil).Obfuscate(&tc.span)
		assert.Equal("Non-parsable SQL query", tc.span.Resource)
		assert.Equal("Non-parsable SQL query", tc.span.Meta["sql.query"])
	}
}

func TestSQLUTF8(t *testing.T) {
	assert := assert.New(t)
	for _, tt := range []struct{ in, out string }{
		{
			"SELECT Codi , Nom_CA AS Nom, Descripció_CAT AS Descripció FROM ProtValAptitud WHERE Vigent=1 ORDER BY Ordre, Codi",
			"SELECT Codi, Nom_CA, Descripció_CAT FROM ProtValAptitud WHERE Vigent = ? ORDER BY Ordre, Codi",
		},
		{
			" SELECT  dbo.Treballadors_ProtCIE_AntecedentsPatologics.IdTreballadorsProtCIE_AntecedentsPatologics,   dbo.ProtCIE.Codi As CodiProtCIE, Treballadors_ProtCIE_AntecedentsPatologics.Año,                              dbo.ProtCIE.Nom_ES, dbo.ProtCIE.Nom_CA  FROM         dbo.Treballadors_ProtCIE_AntecedentsPatologics  WITH (NOLOCK)  INNER JOIN                       dbo.ProtCIE  WITH (NOLOCK)  ON dbo.Treballadors_ProtCIE_AntecedentsPatologics.CodiProtCIE = dbo.ProtCIE.Codi  WHERE Treballadors_ProtCIE_AntecedentsPatologics.IdTreballador =  12345 ORDER BY   Treballadors_ProtCIE_AntecedentsPatologics.Año DESC, dbo.ProtCIE.Codi ",
			"SELECT dbo.Treballadors_ProtCIE_AntecedentsPatologics.IdTreballadorsProtCIE_AntecedentsPatologics, dbo.ProtCIE.Codi, Treballadors_ProtCIE_AntecedentsPatologics.Año, dbo.ProtCIE.Nom_ES, dbo.ProtCIE.Nom_CA FROM dbo.Treballadors_ProtCIE_AntecedentsPatologics WITH ( NOLOCK ) INNER JOIN dbo.ProtCIE WITH ( NOLOCK ) ON dbo.Treballadors_ProtCIE_AntecedentsPatologics.CodiProtCIE = dbo.ProtCIE.Codi WHERE Treballadors_ProtCIE_AntecedentsPatologics.IdTreballador = ? ORDER BY Treballadors_ProtCIE_AntecedentsPatologics.Año DESC, dbo.ProtCIE.Codi",
		},
		{
			"select  top 100 percent  IdTrebEmpresa as [IdTrebEmpresa], CodCli as [Client], NOMEMP as [Nom Client], Baixa as [Baixa], CASE WHEN IdCentreTreball IS NULL THEN '-' ELSE  CONVERT(VARCHAR(8),IdCentreTreball) END as [Id Centre],  CASE WHEN NOMESTAB IS NULL THEN '-' ELSE NOMESTAB END  as [Nom Centre],  TIPUS as [Tipus Lloc], CASE WHEN IdLloc IS NULL THEN '-' ELSE  CONVERT(VARCHAR(8),IdLloc) END  as [Id Lloc],  CASE WHEN NomLlocComplert IS NULL THEN '-' ELSE NomLlocComplert END  as [Lloc Treball],  CASE WHEN DesLloc IS NULL THEN '-' ELSE DesLloc END  as [Descripció], IdLlocTreballUnic as [Id Únic]  From ( SELECT    '-' AS TIPUS,  dbo.Treb_Empresa.IdTrebEmpresa, dbo.Treb_Empresa.IdTreballador, dbo.Treb_Empresa.CodCli, dbo.Clients.NOMEMP,   dbo.Treb_Empresa.Baixa,                      dbo.Treb_Empresa.IdCentreTreball, dbo.Cli_Establiments.NOMESTAB, null AS IdLloc,                        null AS NomLlocComplert, dbo.Treb_Empresa.DataInici,                        dbo.Treb_Empresa.DataFi, CASE WHEN dbo.Treb_Empresa.DesLloc IS NULL THEN '' ELSE dbo.Treb_Empresa.DesLloc END DesLloc, dbo.Treb_Empresa.IdLlocTreballUnic FROM         dbo.Clients  WITH (NOLOCK) INNER JOIN                       dbo.Treb_Empresa  WITH (NOLOCK) ON dbo.Clients.CODCLI = dbo.Treb_Empresa.CodCli LEFT OUTER JOIN                       dbo.Cli_Establiments  WITH (NOLOCK) ON dbo.Cli_Establiments.Id_ESTAB_CLI = dbo.Treb_Empresa.IdCentreTreball AND                        dbo.Cli_Establiments.CODCLI = dbo.Treb_Empresa.CodCli WHERE     dbo.Treb_Empresa.IdTreballador = 64376 AND Treb_Empresa.IdTecEIRLLlocTreball IS NULL AND IdMedEIRLLlocTreball IS NULL AND IdLlocTreballTemporal IS NULL  UNION ALL SELECT    'AV. RIESGO' AS TIPUS,  dbo.Treb_Empresa.IdTrebEmpresa, dbo.Treb_Empresa.IdTreballador, dbo.Treb_Empresa.CodCli, dbo.Clients.NOMEMP, dbo.Treb_Empresa.Baixa,                       dbo.Treb_Empresa.IdCentreTreball, dbo.Cli_Establiments.NOMESTAB, dbo.Treb_Empresa.IdTecEIRLLlocTreball AS IdLloc,                        dbo.fn_NomLlocComposat(dbo.Treb_Empresa.IdTecEIRLLlocTreball) AS NomLlocComplert, dbo.Treb_Empresa.DataInici,                        dbo.Treb_Empresa.DataFi, CASE WHEN dbo.Treb_Empresa.DesLloc IS NULL THEN '' ELSE dbo.Treb_Empresa.DesLloc END DesLloc, dbo.Treb_Empresa.IdLlocTreballUnic FROM         dbo.Clients  WITH (NOLOCK) INNER JOIN                       dbo.Treb_Empresa  WITH (NOLOCK) ON dbo.Clients.CODCLI = dbo.Treb_Empresa.CodCli LEFT OUTER JOIN                       dbo.Cli_Establiments  WITH (NOLOCK) ON dbo.Cli_Establiments.Id_ESTAB_CLI = dbo.Treb_Empresa.IdCentreTreball AND                        dbo.Cli_Establiments.CODCLI = dbo.Treb_Empresa.CodCli WHERE     (dbo.Treb_Empresa.IdTreballador = 64376) AND (NOT (dbo.Treb_Empresa.IdTecEIRLLlocTreball IS NULL))  UNION ALL SELECT     'EXTERNA' AS TIPUS,  dbo.Treb_Empresa.IdTrebEmpresa, dbo.Treb_Empresa.IdTreballador, dbo.Treb_Empresa.CodCli, dbo.Clients.NOMEMP,  dbo.Treb_Empresa.Baixa,                      dbo.Treb_Empresa.IdCentreTreball, dbo.Cli_Establiments.NOMESTAB, dbo.Treb_Empresa.IdMedEIRLLlocTreball AS IdLloc,                        dbo.fn_NomMedEIRLLlocComposat(dbo.Treb_Empresa.IdMedEIRLLlocTreball) AS NomLlocComplert,  dbo.Treb_Empresa.DataInici,                        dbo.Treb_Empresa.DataFi, CASE WHEN dbo.Treb_Empresa.DesLloc IS NULL THEN '' ELSE dbo.Treb_Empresa.DesLloc END DesLloc, dbo.Treb_Empresa.IdLlocTreballUnic FROM         dbo.Clients  WITH (NOLOCK) INNER JOIN                       dbo.Treb_Empresa  WITH (NOLOCK) ON dbo.Clients.CODCLI = dbo.Treb_Empresa.CodCli LEFT OUTER JOIN                       dbo.Cli_Establiments  WITH (NOLOCK) ON dbo.Cli_Establiments.Id_ESTAB_CLI = dbo.Treb_Empresa.IdCentreTreball AND                        dbo.Cli_Establiments.CODCLI = dbo.Treb_Empresa.CodCli WHERE     (dbo.Treb_Empresa.IdTreballador = 64376) AND (Treb_Empresa.IdTecEIRLLlocTreball IS NULL) AND (NOT (dbo.Treb_Empresa.IdMedEIRLLlocTreball IS NULL))  UNION ALL SELECT     'TEMPORAL' AS TIPUS,  dbo.Treb_Empresa.IdTrebEmpresa, dbo.Treb_Empresa.IdTreballador, dbo.Treb_Empresa.CodCli, dbo.Clients.NOMEMP, dbo.Treb_Empresa.Baixa,                       dbo.Treb_Empresa.IdCentreTreball, dbo.Cli_Establiments.NOMESTAB, dbo.Treb_Empresa.IdLlocTreballTemporal AS IdLloc,                       dbo.Lloc_Treball_Temporal.NomLlocTreball AS NomLlocComplert,  dbo.Treb_Empresa.DataInici,                        dbo.Treb_Empresa.DataFi, CASE WHEN dbo.Treb_Empresa.DesLloc IS NULL THEN '' ELSE dbo.Treb_Empresa.DesLloc END DesLloc, dbo.Treb_Empresa.IdLlocTreballUnic FROM         dbo.Clients  WITH (NOLOCK) INNER JOIN                       dbo.Treb_Empresa  WITH (NOLOCK) ON dbo.Clients.CODCLI = dbo.Treb_Empresa.CodCli INNER JOIN                       dbo.Lloc_Treball_Temporal  WITH (NOLOCK) ON dbo.Treb_Empresa.IdLlocTreballTemporal = dbo.Lloc_Treball_Temporal.IdLlocTreballTemporal LEFT OUTER JOIN                       dbo.Cli_Establiments  WITH (NOLOCK) ON dbo.Cli_Establiments.Id_ESTAB_CLI = dbo.Treb_Empresa.IdCentreTreball AND                        dbo.Cli_Establiments.CODCLI = dbo.Treb_Empresa.CodCli WHERE     dbo.Treb_Empresa.IdTreballador = 64376 AND Treb_Empresa.IdTecEIRLLlocTreball IS NULL AND IdMedEIRLLlocTreball IS NULL ) as taula  Where 1=0 ",
			"select top ? percent IdTrebEmpresa, CodCli, NOMEMP, Baixa, CASE WHEN IdCentreTreball IS ? THEN ? ELSE CONVERT ( VARCHAR ( ? ) IdCentreTreball ) END, CASE WHEN NOMESTAB IS ? THEN ? ELSE NOMESTAB END, TIPUS, CASE WHEN IdLloc IS ? THEN ? ELSE CONVERT ( VARCHAR ( ? ) IdLloc ) END, CASE WHEN NomLlocComplert IS ? THEN ? ELSE NomLlocComplert END, CASE WHEN DesLloc IS ? THEN ? ELSE DesLloc END, IdLlocTreballUnic From ( SELECT ?, dbo.Treb_Empresa.IdTrebEmpresa, dbo.Treb_Empresa.IdTreballador, dbo.Treb_Empresa.CodCli, dbo.Clients.NOMEMP, dbo.Treb_Empresa.Baixa, dbo.Treb_Empresa.IdCentreTreball, dbo.Cli_Establiments.NOMESTAB, ?, ?, dbo.Treb_Empresa.DataInici, dbo.Treb_Empresa.DataFi, CASE WHEN dbo.Treb_Empresa.DesLloc IS ? THEN ? ELSE dbo.Treb_Empresa.DesLloc END DesLloc, dbo.Treb_Empresa.IdLlocTreballUnic FROM dbo.Clients WITH ( NOLOCK ) INNER JOIN dbo.Treb_Empresa WITH ( NOLOCK ) ON dbo.Clients.CODCLI = dbo.Treb_Empresa.CodCli LEFT OUTER JOIN dbo.Cli_Establiments WITH ( NOLOCK ) ON dbo.Cli_Establiments.Id_ESTAB_CLI = dbo.Treb_Empresa.IdCentreTreball AND dbo.Cli_Establiments.CODCLI = dbo.Treb_Empresa.CodCli WHERE dbo.Treb_Empresa.IdTreballador = ? AND Treb_Empresa.IdTecEIRLLlocTreball IS ? AND IdMedEIRLLlocTreball IS ? AND IdLlocTreballTemporal IS ? UNION ALL SELECT ?, dbo.Treb_Empresa.IdTrebEmpresa, dbo.Treb_Empresa.IdTreballador, dbo.Treb_Empresa.CodCli, dbo.Clients.NOMEMP, dbo.Treb_Empresa.Baixa, dbo.Treb_Empresa.IdCentreTreball, dbo.Cli_Establiments.NOMESTAB, dbo.Treb_Empresa.IdTecEIRLLlocTreball, dbo.fn_NomLlocComposat ( dbo.Treb_Empresa.IdTecEIRLLlocTreball ), dbo.Treb_Empresa.DataInici, dbo.Treb_Empresa.DataFi, CASE WHEN dbo.Treb_Empresa.DesLloc IS ? THEN ? ELSE dbo.Treb_Empresa.DesLloc END DesLloc, dbo.Treb_Empresa.IdLlocTreballUnic FROM dbo.Clients WITH ( NOLOCK ) INNER JOIN dbo.Treb_Empresa WITH ( NOLOCK ) ON dbo.Clients.CODCLI = dbo.Treb_Empresa.CodCli LEFT OUTER JOIN dbo.Cli_Establiments WITH ( NOLOCK ) ON dbo.Cli_Establiments.Id_ESTAB_CLI = dbo.Treb_Empresa.IdCentreTreball AND dbo.Cli_Establiments.CODCLI = dbo.Treb_Empresa.CodCli WHERE ( dbo.Treb_Empresa.IdTreballador = ? ) AND ( NOT ( dbo.Treb_Empresa.IdTecEIRLLlocTreball IS ? ) ) UNION ALL SELECT ?, dbo.Treb_Empresa.IdTrebEmpresa, dbo.Treb_Empresa.IdTreballador, dbo.Treb_Empresa.CodCli, dbo.Clients.NOMEMP, dbo.Treb_Empresa.Baixa, dbo.Treb_Empresa.IdCentreTreball, dbo.Cli_Establiments.NOMESTAB, dbo.Treb_Empresa.IdMedEIRLLlocTreball, dbo.fn_NomMedEIRLLlocComposat ( dbo.Treb_Empresa.IdMedEIRLLlocTreball ), dbo.Treb_Empresa.DataInici, dbo.Treb_Empresa.DataFi, CASE WHEN dbo.Treb_Empresa.DesLloc IS ? THEN ? ELSE dbo.Treb_Empresa.DesLloc END DesLloc, dbo.Treb_Empresa.IdLlocTreballUnic FROM dbo.Clients WITH ( NOLOCK ) INNER JOIN dbo.Treb_Empresa WITH ( NOLOCK ) ON dbo.Clients.CODCLI = dbo.Treb_Empresa.CodCli LEFT OUTER JOIN dbo.Cli_Establiments WITH ( NOLOCK ) ON dbo.Cli_Establiments.Id_ESTAB_CLI = dbo.Treb_Empresa.IdCentreTreball AND dbo.Cli_Establiments.CODCLI = dbo.Treb_Empresa.CodCli WHERE ( dbo.Treb_Empresa.IdTreballador = ? ) AND ( Treb_Empresa.IdTecEIRLLlocTreball IS ? ) AND ( NOT ( dbo.Treb_Empresa.IdMedEIRLLlocTreball IS ? ) ) UNION ALL SELECT ?, dbo.Treb_Empresa.IdTrebEmpresa, dbo.Treb_Empresa.IdTreballador, dbo.Treb_Empresa.CodCli, dbo.Clients.NOMEMP, dbo.Treb_Empresa.Baixa, dbo.Treb_Empresa.IdCentreTreball, dbo.Cli_Establiments.NOMESTAB, dbo.Treb_Empresa.IdLlocTreballTemporal, dbo.Lloc_Treball_Temporal.NomLlocTreball, dbo.Treb_Empresa.DataInici, dbo.Treb_Empresa.DataFi, CASE WHEN dbo.Treb_Empresa.DesLloc IS ? THEN ? ELSE dbo.Treb_Empresa.DesLloc END DesLloc, dbo.Treb_Empresa.IdLlocTreballUnic FROM dbo.Clients WITH ( NOLOCK ) INNER JOIN dbo.Treb_Empresa WITH ( NOLOCK ) ON dbo.Clients.CODCLI = dbo.Treb_Empresa.CodCli INNER JOIN dbo.Lloc_Treball_Temporal WITH ( NOLOCK ) ON dbo.Treb_Empresa.IdLlocTreballTemporal = dbo.Lloc_Treball_Temporal.IdLlocTreballTemporal LEFT OUTER JOIN dbo.Cli_Establiments WITH ( NOLOCK ) ON dbo.Cli_Establiments.Id_ESTAB_CLI = dbo.Treb_Empresa.IdCentreTreball AND dbo.Cli_Establiments.CODCLI = dbo.Treb_Empresa.CodCli WHERE dbo.Treb_Empresa.IdTreballador = ? AND Treb_Empresa.IdTecEIRLLlocTreball IS ? AND IdMedEIRLLlocTreball IS ? ) Where ? = ?",
		},
		{
			"select  IdHistLabAnt as [IdHistLabAnt], IdTreballador as [IdTreballador], Empresa as [Professió], Anys as [Anys],  Riscs as [Riscos], Nom_CA AS [Prot CNO], Nom_ES as [Prot CNO Altre Idioma]   From ( SELECT     dbo.Treb_HistAnt.IdHistLabAnt, dbo.Treb_HistAnt.IdTreballador,           dbo.Treb_HistAnt.Empresa, dbo.Treb_HistAnt.Anys, dbo.Treb_HistAnt.Riscs, dbo.Treb_HistAnt.CodiProtCNO,           dbo.ProtCNO.Nom_ES, dbo.ProtCNO.Nom_CA  FROM     dbo.Treb_HistAnt  WITH (NOLOCK) LEFT OUTER JOIN                       dbo.ProtCNO  WITH (NOLOCK) ON dbo.Treb_HistAnt.CodiProtCNO = dbo.ProtCNO.Codi  Where  dbo.Treb_HistAnt.IdTreballador = 12345 ) as taula ",
			"select IdHistLabAnt, IdTreballador, Empresa, Anys, Riscs, Nom_CA, Nom_ES From ( SELECT dbo.Treb_HistAnt.IdHistLabAnt, dbo.Treb_HistAnt.IdTreballador, dbo.Treb_HistAnt.Empresa, dbo.Treb_HistAnt.Anys, dbo.Treb_HistAnt.Riscs, dbo.Treb_HistAnt.CodiProtCNO, dbo.ProtCNO.Nom_ES, dbo.ProtCNO.Nom_CA FROM dbo.Treb_HistAnt WITH ( NOLOCK ) LEFT OUTER JOIN dbo.ProtCNO WITH ( NOLOCK ) ON dbo.Treb_HistAnt.CodiProtCNO = dbo.ProtCNO.Codi Where dbo.Treb_HistAnt.IdTreballador = ? )",
		},
		{
			"SELECT     Cli_Establiments.CODCLI, Cli_Establiments.Id_ESTAB_CLI As [Código Centro Trabajo], Cli_Establiments.CODIGO_CENTRO_AXAPTA As [Código C. Axapta],  Cli_Establiments.NOMESTAB As [Nombre],                                 Cli_Establiments.ADRECA As [Dirección], Cli_Establiments.CodPostal As [Código Postal], Cli_Establiments.Poblacio as [Población], Cli_Establiments.Provincia,                                Cli_Establiments.TEL As [Tel],  Cli_Establiments.EMAIL As [EMAIL],                                Cli_Establiments.PERS_CONTACTE As [Contacto], Cli_Establiments.PERS_CONTACTE_CARREC As [Cargo Contacto], Cli_Establiments.NumTreb As [Plantilla],                                Cli_Establiments.Localitzacio As [Localización], Tipus_Activitat.CNAE, Tipus_Activitat.Nom_ES As [Nombre Actividad], ACTIVO AS [Activo]                        FROM         Cli_Establiments LEFT OUTER JOIN                                    Tipus_Activitat ON Cli_Establiments.Id_ACTIVITAT = Tipus_Activitat.IdActivitat                        Where CODCLI = '01234' AND CENTRE_CORRECTE = 3 AND ACTIVO = 5                        ORDER BY Cli_Establiments.CODIGO_CENTRO_AXAPTA ",
			"SELECT Cli_Establiments.CODCLI, Cli_Establiments.Id_ESTAB_CLI, Cli_Establiments.CODIGO_CENTRO_AXAPTA, Cli_Establiments.NOMESTAB, Cli_Establiments.ADRECA, Cli_Establiments.CodPostal, Cli_Establiments.Poblacio, Cli_Establiments.Provincia, Cli_Establiments.TEL, Cli_Establiments.EMAIL, Cli_Establiments.PERS_CONTACTE, Cli_Establiments.PERS_CONTACTE_CARREC, Cli_Establiments.NumTreb, Cli_Establiments.Localitzacio, Tipus_Activitat.CNAE, Tipus_Activitat.Nom_ES, ACTIVO FROM Cli_Establiments LEFT OUTER JOIN Tipus_Activitat ON Cli_Establiments.Id_ACTIVITAT = Tipus_Activitat.IdActivitat Where CODCLI = ? AND CENTRE_CORRECTE = ? AND ACTIVO = ? ORDER BY Cli_Establiments.CODIGO_CENTRO_AXAPTA",
		},
		{
			"select * from `構わない`;",
			"select * from 構わない",
		},
		{
			"select * from names where name like '�����';",
			"select * from names where name like ?",
		},
		{
			"select replacement from table where replacement = 'i�n�t�e��rspersed';",
			"select replacement from table where replacement = ?",
		},
		{
			"SELECT ('\ufffd');",
			"SELECT ( ? )",
		},
	} {
		t.Run("", func(t *testing.T) {
			oq, err := NewObfuscator(nil).ObfuscateSQLString(tt.in)
			assert.NoError(err)
			assert.Equal(tt.out, oq.Query)
		})
	}
}

func TestSQLTableNames(t *testing.T) {
	t.Run("on", func(t *testing.T) {
		defer testutil.WithFeatures("table_names")()

		span := &pb.Span{
			Resource: "SELECT * FROM users WHERE id = 42",
			Type:     "sql",
		}
		NewObfuscator(nil).Obfuscate(span)
		assert.Equal(t, "users", span.Meta["sql.tables"])

	})

	t.Run("off", func(t *testing.T) {
		span := &pb.Span{
			Resource: "SELECT * FROM users WHERE id = 42",
			Type:     "sql",
		}
		NewObfuscator(nil).Obfuscate(span)
		assert.Empty(t, span.Meta["sql.tables"])
	})
}

func TestSQLQuantizeTableNames(t *testing.T) {
	t.Run("on", func(t *testing.T) {
		defer testutil.WithFeatures("quantize_sql_tables")()

		for _, tt := range []struct {
			query      string
			obfuscated string
		}{
			{
				"REPLACE INTO sales_2019_07_01 (`itemID`, `date`, `qty`, `price`) VALUES ((SELECT itemID FROM item1001 WHERE `sku` = [sku]), CURDATE(), [qty], 0.00)",
				"REPLACE INTO sales_?_?_? ( itemID, date, qty, price ) VALUES ( ( SELECT itemID FROM item? WHERE sku = [ sku ] ), CURDATE ( ), [ qty ], ? )",
			},
		} {
			t.Run("", func(t *testing.T) {
				assert := assert.New(t)
				oq, err := NewObfuscator(nil).ObfuscateSQLString(tt.query)
				assert.NoError(err)
				assert.Empty(oq.TablesCSV)
				assert.Equal(tt.obfuscated, oq.Query)
			})
		}
	})

	t.Run("off", func(t *testing.T) {
		for _, tt := range []struct {
			query      string
			obfuscated string
		}{
			{
				"REPLACE INTO sales_2019_07_01 (`itemID`, `date`, `qty`, `price`) VALUES ((SELECT itemID FROM item1001 WHERE `sku` = [sku]), CURDATE(), [qty], 0.00)",
				"REPLACE INTO sales_2019_07_01 ( itemID, date, qty, price ) VALUES ( ( SELECT itemID FROM item1001 WHERE sku = [ sku ] ), CURDATE ( ), [ qty ], ? )",
			},
		} {
			t.Run("", func(t *testing.T) {
				assert := assert.New(t)
				oq, err := NewObfuscator(nil).ObfuscateSQLString(tt.query)
				assert.NoError(err)
				assert.Empty(oq.TablesCSV)
				assert.Equal(tt.obfuscated, oq.Query)
			})
		}
	})
}

func TestSQLTableFinderAndQuantizeTableNames(t *testing.T) {
	t.Run("on", func(t *testing.T) {
		defer testutil.WithFeatures("table_names,quantize_sql_tables")()

		for _, tt := range []struct {
			query      string
			tables     string
			obfuscated string
		}{
			{
				"select * from users where id = 42",
				"users",
				"select * from users where id = ?",
			},
			{
				"select * from `backslashes` where id = 42",
				"backslashes",
				"select * from backslashes where id = ?",
			},
			{
				`select * from "double-quotes" where id = 42`,
				"double-quotes",
				`select * from double-quotes where id = ?`,
			},
			{
				"SELECT host, status FROM ec2_status WHERE org_id = 42",
				"ec2_status",
				"SELECT host, status FROM ec?_status WHERE org_id = ?",
			},
			{
				"SELECT * FROM (SELECT * FROM nested_table)",
				"nested_table",
				"SELECT * FROM ( SELECT * FROM nested_table )",
			},
			{
				"   -- get user \n--\n select * \n   from users \n    where\n       id = 214325346    ",
				"users",
				"select * from users where id = ?",
			},
			{
				"SELECT articles.* FROM articles WHERE articles.id = 1 LIMIT 1, 20",
				"articles",
				"SELECT articles.* FROM articles WHERE articles.id = ? LIMIT ?",
			},
			{
				"UPDATE user_dash_pref SET json_prefs = %(json_prefs)s, modified = '2015-08-27 22:10:32.492912' WHERE user_id = %(user_id)s AND url = %(url)s",
				"user_dash_pref",
				"UPDATE user_dash_pref SET json_prefs = ? modified = ? WHERE user_id = ? AND url = ?",
			},
			{
				"SELECT DISTINCT host.id AS host_id FROM host JOIN host_alias ON host_alias.host_id = host.id WHERE host.org_id = %(org_id_1)s AND host.name NOT IN (%(name_1)s) AND host.name IN (%(name_2)s, %(name_3)s, %(name_4)s, %(name_5)s)",
				"host,host_alias",
				"SELECT DISTINCT host.id FROM host JOIN host_alias ON host_alias.host_id = host.id WHERE host.org_id = ? AND host.name NOT IN ( ? ) AND host.name IN ( ? )",
			},
			{
				`update Orders set created = "2019-05-24 00:26:17", gross = 30.28, payment_type = "eventbrite", mg_fee = "3.28", fee_collected = "3.28", event = 59366262, status = "10", survey_type = 'direct', tx_time_limit = 480, invite = "", ip_address = "69.215.148.82", currency = 'USD', gross_USD = "30.28", tax_USD = 0.00, journal_activity_id = 4044659812798558774, eb_tax = 0.00, eb_tax_USD = 0.00, cart_uuid = "160b450e7df511e9810e0a0c06de92f8", changed = '2019-05-24 00:26:17' where id = ?`,
				"Orders",
				`update Orders set created = ? gross = ? payment_type = ? mg_fee = ? fee_collected = ? event = ? status = ? survey_type = ? tx_time_limit = ? invite = ? ip_address = ? currency = ? gross_USD = ? tax_USD = ? journal_activity_id = ? eb_tax = ? eb_tax_USD = ? cart_uuid = ? changed = ? where id = ?`,
			},
			{
				"SELECT * FROM clients WHERE (clients.first_name = 'Andy') LIMIT 1 BEGIN INSERT INTO owners (created_at, first_name, locked, orders_count, updated_at) VALUES ('2011-08-30 05:22:57', 'Andy', 1, NULL, '2011-08-30 05:22:57') COMMIT",
				"clients,owners",
				"SELECT * FROM clients WHERE ( clients.first_name = ? ) LIMIT ? BEGIN INSERT INTO owners ( created_at, first_name, locked, orders_count, updated_at ) VALUES ( ? ) COMMIT",
			},
			{
				"DELETE FROM table WHERE table.a=1",
				"table",
				"DELETE FROM table WHERE table.a = ?",
			},
			{
				"SELECT wp_woocommerce_order_items.order_id FROM wp_woocommerce_order_items LEFT JOIN ( SELECT meta_value FROM wp_postmeta WHERE meta_key = ? ) ON wp_woocommerce_order_items.order_id = a.post_id WHERE wp_woocommerce_order_items.order_id = ?",
				"wp_woocommerce_order_items,wp_postmeta",
				"SELECT wp_woocommerce_order_items.order_id FROM wp_woocommerce_order_items LEFT JOIN ( SELECT meta_value FROM wp_postmeta WHERE meta_key = ? ) ON wp_woocommerce_order_items.order_id = a.post_id WHERE wp_woocommerce_order_items.order_id = ?",
			},
			{
				"REPLACE INTO sales_2019_07_01 (`itemID`, `date`, `qty`, `price`) VALUES ((SELECT itemID FROM item1001 WHERE `sku` = [sku]), CURDATE(), [qty], 0.00)",
				"sales_2019_07_01,item1001",
				"REPLACE INTO sales_?_?_? ( itemID, date, qty, price ) VALUES ( ( SELECT itemID FROM item? WHERE sku = [ sku ] ), CURDATE ( ), [ qty ], ? )",
			},
		} {
			t.Run("", func(t *testing.T) {
				assert := assert.New(t)
				oq, err := NewObfuscator(nil).ObfuscateSQLString(tt.query)
				assert.NoError(err)
				assert.Equal(tt.tables, oq.TablesCSV)
				assert.Equal(tt.obfuscated, oq.Query)
			})
		}
	})

	t.Run("off", func(t *testing.T) {
		oq, err := NewObfuscator(nil).ObfuscateSQLString("DELETE FROM table WHERE table.a=1")
		assert.NoError(t, err)
		assert.Empty(t, oq.TablesCSV)
	})
}

func TestSQLQuantizer(t *testing.T) {
	cases := []sqlTestCase{
		{
			"select * from users where id = 42",
			"select * from users where id = ?",
		},
		{
			"select * from users where float = .43422",
			"select * from users where float = ?",
		},
		{
			"SELECT host, status FROM ec2_status WHERE org_id = 42",
			"SELECT host, status FROM ec2_status WHERE org_id = ?",
		},
		{
			"SELECT host, status FROM ec2_status WHERE org_id=42",
			"SELECT host, status FROM ec2_status WHERE org_id = ?",
		},
		{
			"-- get user \n--\n select * \n   from users \n    where\n       id = 214325346",
			"select * from users where id = ?",
		},
		{
			"SELECT * FROM `host` WHERE `id` IN (42, 43) /*comment with parameters,host:localhost,url:controller#home,id:FF005:00CAA*/",
			"SELECT * FROM host WHERE id IN ( ? )",
		},
		{
			"SELECT `host`.`address` FROM `host` WHERE org_id=42",
			"SELECT host . address FROM host WHERE org_id = ?",
		},
		{
			`SELECT "host"."address" FROM "host" WHERE org_id=42`,
			`SELECT host . address FROM host WHERE org_id = ?`,
		},
		{
			`SELECT * FROM host WHERE id IN (42, 43) /*
			multiline comment with parameters,
			host:localhost,url:controller#home,id:FF005:00CAA
			*/`,
			"SELECT * FROM host WHERE id IN ( ? )",
		},
		{
			"UPDATE user_dash_pref SET json_prefs = %(json_prefs)s, modified = '2015-08-27 22:10:32.492912' WHERE user_id = %(user_id)s AND url = %(url)s",
			"UPDATE user_dash_pref SET json_prefs = ? modified = ? WHERE user_id = ? AND url = ?"},
		{
			"SELECT DISTINCT host.id AS host_id FROM host JOIN host_alias ON host_alias.host_id = host.id WHERE host.org_id = %(org_id_1)s AND host.name NOT IN (%(name_1)s) AND host.name IN (%(name_2)s, %(name_3)s, %(name_4)s, %(name_5)s)",
			"SELECT DISTINCT host.id FROM host JOIN host_alias ON host_alias.host_id = host.id WHERE host.org_id = ? AND host.name NOT IN ( ? ) AND host.name IN ( ? )",
		},
		{
			"SELECT org_id, metric_key FROM metrics_metadata WHERE org_id = %(org_id)s AND metric_key = ANY(array[75])",
			"SELECT org_id, metric_key FROM metrics_metadata WHERE org_id = ? AND metric_key = ANY ( array [ ? ] )",
		},
		{
			"SELECT org_id, metric_key   FROM metrics_metadata   WHERE org_id = %(org_id)s AND metric_key = ANY(array[21, 25, 32])",
			"SELECT org_id, metric_key FROM metrics_metadata WHERE org_id = ? AND metric_key = ANY ( array [ ? ] )",
		},
		{
			"SELECT articles.* FROM articles WHERE articles.id = 1 LIMIT 1",
			"SELECT articles.* FROM articles WHERE articles.id = ? LIMIT ?",
		},

		{
			"SELECT articles.* FROM articles WHERE articles.id = 1 LIMIT 1, 20",
			"SELECT articles.* FROM articles WHERE articles.id = ? LIMIT ?",
		},
		{
			"SELECT articles.* FROM articles WHERE articles.id = 1 LIMIT 1, 20;",
			"SELECT articles.* FROM articles WHERE articles.id = ? LIMIT ?",
		},
		{
			"SELECT articles.* FROM articles WHERE articles.id = 1 LIMIT 15,20;",
			"SELECT articles.* FROM articles WHERE articles.id = ? LIMIT ?",
		},
		{
			"SELECT articles.* FROM articles WHERE articles.id = 1 LIMIT 1;",
			"SELECT articles.* FROM articles WHERE articles.id = ? LIMIT ?",
		},
		{
			"SELECT articles.* FROM articles WHERE (articles.created_at BETWEEN '2016-10-31 23:00:00.000000' AND '2016-11-01 23:00:00.000000')",
			"SELECT articles.* FROM articles WHERE ( articles.created_at BETWEEN ? AND ? )",
		},
		{
			"SELECT articles.* FROM articles WHERE (articles.created_at BETWEEN $1 AND $2)",
			"SELECT articles.* FROM articles WHERE ( articles.created_at BETWEEN ? AND ? )",
		},
		{
			"SELECT articles.* FROM articles WHERE (articles.published != true)",
			"SELECT articles.* FROM articles WHERE ( articles.published != ? )",
		},
		{
			"SELECT articles.* FROM articles WHERE (title = 'guides.rubyonrails.org')",
			"SELECT articles.* FROM articles WHERE ( title = ? )",
		},
		{
			"SELECT articles.* FROM articles WHERE ( title = ? ) AND ( author = ? )",
			"SELECT articles.* FROM articles WHERE ( title = ? ) AND ( author = ? )",
		},
		{
			"SELECT articles.* FROM articles WHERE ( title = :title )",
			"SELECT articles.* FROM articles WHERE ( title = :title )",
		},
		{
			"SELECT articles.* FROM articles WHERE ( title = @title )",
			"SELECT articles.* FROM articles WHERE ( title = @title )",
		},
		{
			"SELECT date(created_at) as ordered_date, sum(price) as total_price FROM orders GROUP BY date(created_at) HAVING sum(price) > 100",
			"SELECT date ( created_at ), sum ( price ) FROM orders GROUP BY date ( created_at ) HAVING sum ( price ) > ?",
		},
		{
			"SELECT * FROM articles WHERE id > 10 ORDER BY id asc LIMIT 20",
			"SELECT * FROM articles WHERE id > ? ORDER BY id asc LIMIT ?",
		},
		{
			"SELECT clients.* FROM clients INNER JOIN posts ON posts.author_id = author.id AND posts.published = 't'",
			"SELECT clients.* FROM clients INNER JOIN posts ON posts.author_id = author.id AND posts.published = ?",
		},
		{
			"SELECT articles.* FROM articles WHERE articles.id IN (1, 3, 5)",
			"SELECT articles.* FROM articles WHERE articles.id IN ( ? )",
		},
		{
			"SELECT * FROM clients WHERE (clients.first_name = 'Andy') LIMIT 1 BEGIN INSERT INTO clients (created_at, first_name, locked, orders_count, updated_at) VALUES ('2011-08-30 05:22:57', 'Andy', 1, NULL, '2011-08-30 05:22:57') COMMIT",
			"SELECT * FROM clients WHERE ( clients.first_name = ? ) LIMIT ? BEGIN INSERT INTO clients ( created_at, first_name, locked, orders_count, updated_at ) VALUES ( ? ) COMMIT",
		},
		{
			"SELECT * FROM clients WHERE (clients.first_name = 'Andy') LIMIT 15, 25 BEGIN INSERT INTO clients (created_at, first_name, locked, orders_count, updated_at) VALUES ('2011-08-30 05:22:57', 'Andy', 1, NULL, '2011-08-30 05:22:57') COMMIT",
			"SELECT * FROM clients WHERE ( clients.first_name = ? ) LIMIT ? BEGIN INSERT INTO clients ( created_at, first_name, locked, orders_count, updated_at ) VALUES ( ? ) COMMIT",
		},
		{
			"SAVEPOINT \"s139956586256192_x1\"",
			"SAVEPOINT ?",
		},
		{
			"INSERT INTO user (id, username) VALUES ('Fred','Smith'), ('John','Smith'), ('Michael','Smith'), ('Robert','Smith');",
			"INSERT INTO user ( id, username ) VALUES ( ? )",
		},
		{
			"CREATE KEYSPACE Excelsior WITH replication = {'class': 'SimpleStrategy', 'replication_factor' : 3};",
			"CREATE KEYSPACE Excelsior WITH replication = ?",
		},
		{
			`SELECT "webcore_page"."id" FROM "webcore_page" WHERE "webcore_page"."slug" = %s ORDER BY "webcore_page"."path" ASC LIMIT 1`,
			"SELECT webcore_page . id FROM webcore_page WHERE webcore_page . slug = ? ORDER BY webcore_page . path ASC LIMIT ?",
		},
		{
			"SELECT server_table.host AS host_id FROM table#.host_tags as server_table WHERE server_table.host_id = 50",
			"SELECT server_table.host FROM table#.host_tags WHERE server_table.host_id = ?",
		},
		{
			`INSERT INTO delayed_jobs (attempts, created_at, failed_at, handler, last_error, locked_at, locked_by, priority, queue, run_at, updated_at) VALUES (0, '2016-12-04 17:09:59', NULL, '--- !ruby/object:Delayed::PerformableMethod\nobject: !ruby/object:Item\n  store:\n  - a simple string\n  - an \'escaped \' string\n  - another \'escaped\' string\n  - 42\n  string: a string with many \\\\\'escapes\\\\\'\nmethod_name: :show_store\nargs: []\n', NULL, NULL, NULL, 0, NULL, '2016-12-04 17:09:59', '2016-12-04 17:09:59')`,
			"INSERT INTO delayed_jobs ( attempts, created_at, failed_at, handler, last_error, locked_at, locked_by, priority, queue, run_at, updated_at ) VALUES ( ? )",
		},
		{
			"SELECT name, pretty_print(address) FROM people;",
			"SELECT name, pretty_print ( address ) FROM people",
		},
		{
			"* SELECT * FROM fake_data(1, 2, 3);",
			"* SELECT * FROM fake_data ( ? )",
		},
		{
			"CREATE FUNCTION add(integer, integer) RETURNS integer\n AS 'select $1 + $2;'\n LANGUAGE SQL\n IMMUTABLE\n RETURNS NULL ON NULL INPUT;",
			"CREATE FUNCTION add ( integer, integer ) RETURNS integer LANGUAGE SQL IMMUTABLE RETURNS ? ON ? INPUT",
		},
		{
			"SELECT * FROM public.table ( array [ ROW ( array [ 'magic', 'foo',",
			"SELECT * FROM public.table ( array [ ROW ( array [ ?",
		},
		{
			"SELECT pg_try_advisory_lock (123) AS t46eef3f025cc27feb31ca5a2d668a09a",
			"SELECT pg_try_advisory_lock ( ? )",
		},
		{
			"INSERT INTO `qual-aa`.issues (alert0 , alert1) VALUES (NULL, NULL)",
			"INSERT INTO qual-aa . issues ( alert0, alert1 ) VALUES ( ? )",
		},
		{
			"INSERT INTO user (id, email, name) VALUES (null, ?, ?)",
			"INSERT INTO user ( id, email, name ) VALUES ( ? )",
		},
		{
			"select * from users where id = 214325346     # This comment continues to the end of line",
			"select * from users where id = ?",
		},
		{
			"select * from users where id = 214325346     -- This comment continues to the end of line",
			"select * from users where id = ?",
		},
		{
			"SELECT * FROM /* this is an in-line comment */ users;",
			"SELECT * FROM users",
		},
		{
			"SELECT /*! STRAIGHT_JOIN */ col1 FROM table1",
			"SELECT col1 FROM table1",
		},
		{
			`DELETE FROM t1
			WHERE s11 > ANY
			(SELECT COUNT(*) /* no hint */ FROM t2
			WHERE NOT EXISTS
			(SELECT * FROM t3
			WHERE ROW(5*t2.s1,77)=
			(SELECT 50,11*s1 FROM t4 UNION SELECT 50,77 FROM
			(SELECT * FROM t5) AS t5)));`,
			"DELETE FROM t1 WHERE s11 > ANY ( SELECT COUNT ( * ) FROM t2 WHERE NOT EXISTS ( SELECT * FROM t3 WHERE ROW ( ? * t2.s1, ? ) = ( SELECT ? * s1 FROM t4 UNION SELECT ? FROM ( SELECT * FROM t5 ) ) ) )",
		},
		{
			"SET @g = 'POLYGON((0 0,10 0,10 10,0 10,0 0),(5 5,7 5,7 7,5 7, 5 5))';",
			"SET @g = ?",
		},
		{
			`SELECT daily_values.*,
                    LEAST((5040000 - @runtot), value) AS value,
                    ` + "(@runtot := @runtot + daily_values.value) AS total FROM (SELECT @runtot:=0) AS n, `daily_values`  WHERE `daily_values`.`subject_id` = 12345 AND `daily_values`.`subject_type` = 'Skippity' AND (daily_values.date BETWEEN '2018-05-09' AND '2018-06-19') HAVING value >= 0 ORDER BY date",
			`SELECT daily_values.*, LEAST ( ( ? - @runtot ), value ), ( @runtot := @runtot + daily_values.value ) FROM ( SELECT @runtot := ? ), daily_values WHERE daily_values . subject_id = ? AND daily_values . subject_type = ? AND ( daily_values.date BETWEEN ? AND ? ) HAVING value >= ? ORDER BY date`,
		},
		{
			`    SELECT
      t1.userid,
      t1.fullname,
      t1.firm_id,
      t2.firmname,
      t1.email,
      t1.location,
      t1.state,
      t1.phone,
      t1.url,
      DATE_FORMAT( t1.lastmod, "%m/%d/%Y %h:%i:%s" ) AS lastmod,
      t1.lastmod AS lastmod_raw,
      t1.user_status,
      t1.pw_expire,
      DATE_FORMAT( t1.pw_expire, "%m/%d/%Y" ) AS pw_expire_date,
      t1.addr1,
      t1.addr2,
      t1.zipcode,
      t1.office_id,
      t1.default_group,
      t3.firm_status,
      t1.title
    FROM
           userdata      AS t1
      LEFT JOIN lawfirm_names AS t2 ON t1.firm_id = t2.firm_id
      LEFT JOIN lawfirms      AS t3 ON t1.firm_id = t3.firm_id
    WHERE
      t1.userid = 'jstein'

  `,
			`SELECT t1.userid, t1.fullname, t1.firm_id, t2.firmname, t1.email, t1.location, t1.state, t1.phone, t1.url, DATE_FORMAT ( t1.lastmod, %m/%d/%Y %h:%i:%s ), t1.lastmod, t1.user_status, t1.pw_expire, DATE_FORMAT ( t1.pw_expire, %m/%d/%Y ), t1.addr1, t1.addr2, t1.zipcode, t1.office_id, t1.default_group, t3.firm_status, t1.title FROM userdata LEFT JOIN lawfirm_names ON t1.firm_id = t2.firm_id LEFT JOIN lawfirms ON t1.firm_id = t3.firm_id WHERE t1.userid = ?`,
		},
		{
			`SELECT [b].[BlogId], [b].[Name]
FROM [Blogs] AS [b]
ORDER BY [b].[Name]`,
			`SELECT [ b ] . [ BlogId ], [ b ] . [ Name ] FROM [ Blogs ] ORDER BY [ b ] . [ Name ]`,
		},
		{
			`SELECT * FROM users WHERE firstname=''`,
			`SELECT * FROM users WHERE firstname = ?`,
		},
		{
			`SELECT * FROM users WHERE firstname=' '`,
			`SELECT * FROM users WHERE firstname = ?`,
		},
		{
			`SELECT * FROM users WHERE firstname=""`,
			`SELECT * FROM users WHERE firstname = ?`,
		},
		{
			`SELECT * FROM users WHERE lastname=" "`,
			`SELECT * FROM users WHERE lastname = ?`,
		},
		{
			`SELECT * FROM users WHERE lastname="	 "`,
			`SELECT * FROM users WHERE lastname = ?`,
		},
		{
			`SELECT [b].[BlogId], [b].[Name]
FROM [Blogs] AS [b
ORDER BY [b].[Name]`,
			`Non-parsable SQL query`,
		},
		{
			`SELECT customer_item_list_id, customer_id FROM customer_item_list WHERE type = wishlist AND customer_id = ? AND visitor_id IS ? UNION SELECT customer_item_list_id, customer_id FROM customer_item_list WHERE type = wishlist AND customer_id IS ? AND visitor_id = "AA0DKTGEM6LRN3WWPZ01Q61E3J7ROX7O" ORDER BY customer_id DESC`,
			"SELECT customer_item_list_id, customer_id FROM customer_item_list WHERE type = wishlist AND customer_id = ? AND visitor_id IS ? UNION SELECT customer_item_list_id, customer_id FROM customer_item_list WHERE type = wishlist AND customer_id IS ? AND visitor_id = ? ORDER BY customer_id DESC",
		},
		{
			`update Orders set created = "2019-05-24 00:26:17", gross = 30.28, payment_type = "eventbrite", mg_fee = "3.28", fee_collected = "3.28", event = 59366262, status = "10", survey_type = 'direct', tx_time_limit = 480, invite = "", ip_address = "69.215.148.82", currency = 'USD', gross_USD = "30.28", tax_USD = 0.00, journal_activity_id = 4044659812798558774, eb_tax = 0.00, eb_tax_USD = 0.00, cart_uuid = "160b450e7df511e9810e0a0c06de92f8", changed = '2019-05-24 00:26:17' where id = ?`,
			`update Orders set created = ? gross = ? payment_type = ? mg_fee = ? fee_collected = ? event = ? status = ? survey_type = ? tx_time_limit = ? invite = ? ip_address = ? currency = ? gross_USD = ? tax_USD = ? journal_activity_id = ? eb_tax = ? eb_tax_USD = ? cart_uuid = ? changed = ? where id = ?`,
		},
		{
			`update Attendees set email = '626837270@qq.com', first_name = "贺新春送猪福加企鹅1054948000领98綵斤", last_name = '王子198442com体验猪多优惠', journal_activity_id = 4246684839261125564, changed = "2019-05-24 00:26:22" where id = 123`,
			`update Attendees set email = ? first_name = ? last_name = ? journal_activity_id = ? changed = ? where id = ?`,
		},
		{
			"SELECT\r\n\t                CodiFormacio\r\n\t                ,DataInici\r\n\t                ,DataFi\r\n\t                ,Tipo\r\n\t                ,CodiTecnicFormador\r\n\t                ,p.nombre AS TutorNombre\r\n\t                ,p.mail AS TutorMail\r\n\t                ,Sessions.Direccio\r\n\t                ,Sessions.NomEmpresa\r\n\t                ,Sessions.Telefon\r\n                FROM\r\n                ----------------------------\r\n                (SELECT\r\n\t                CodiFormacio\r\n\t                ,case\r\n\t                   when ModalitatSessio = '1' then 'Presencial'--Teoria\r\n\t                   when ModalitatSessio = '2' then 'Presencial'--Practica\r\n\t                   when ModalitatSessio = '3' then 'Online'--Tutoria\r\n                       when ModalitatSessio = '4' then 'Presencial'--Examen\r\n\t                   ELSE 'Presencial'\r\n\t                end as Tipo\r\n\t                ,ModalitatSessio\r\n\t                ,DataInici\r\n\t                ,DataFi\r\n                     ,NomEmpresa\r\n\t                ,Telefon\r\n\t                ,CodiTecnicFormador\r\n\t                ,CASE\r\n\t                   WHEn EsAltres = 1 then FormacioLlocImparticioDescripcio\r\n\t                   else Adreca + ' - ' + CodiPostal + ' ' + Poblacio\r\n\t                end as Direccio\r\n\t\r\n                FROM Consultas.dbo.View_AsActiva__FormacioSessions_InfoLlocImparticio) AS Sessions\r\n                ----------------------------------------\r\n                LEFT JOIN Consultas.dbo.View_AsActiva_Operari AS o\r\n\t                ON o.CodiOperari = Sessions.CodiTecnicFormador\r\n                LEFT JOIN MainAPP.dbo.persona AS p\r\n\t                ON 'preven\\' + o.codioperari = p.codi\r\n                WHERE Sessions.CodiFormacio = 'F00000017898'",
			`SELECT CodiFormacio, DataInici, DataFi, Tipo, CodiTecnicFormador, p.nombre, p.mail, Sessions.Direccio, Sessions.NomEmpresa, Sessions.Telefon FROM ( SELECT CodiFormacio, case when ModalitatSessio = ? then ? when ModalitatSessio = ? then ? when ModalitatSessio = ? then ? when ModalitatSessio = ? then ? ELSE ? end, ModalitatSessio, DataInici, DataFi, NomEmpresa, Telefon, CodiTecnicFormador, CASE WHEn EsAltres = ? then FormacioLlocImparticioDescripcio else Adreca + ? + CodiPostal + ? + Poblacio end FROM Consultas.dbo.View_AsActiva__FormacioSessions_InfoLlocImparticio ) LEFT JOIN Consultas.dbo.View_AsActiva_Operari ON o.CodiOperari = Sessions.CodiTecnicFormador LEFT JOIN MainAPP.dbo.persona ON ? + o.codioperari = p.codi WHERE Sessions.CodiFormacio = ?`,
		},
		{
			`SELECT * FROM foo LEFT JOIN bar ON 'backslash\' = foo.b WHERE foo.name = 'String'`,
			"SELECT * FROM foo LEFT JOIN bar ON ? = foo.b WHERE foo.name = ?",
		},
		{
			`SELECT * FROM foo LEFT JOIN bar ON 'backslash\' = foo.b LEFT JOIN bar2 ON 'backslash2\' = foo.b2 WHERE foo.name = 'String'`,
			"SELECT * FROM foo LEFT JOIN bar ON ? = foo.b LEFT JOIN bar2 ON ? = foo.b2 WHERE foo.name = ?",
		},
		{
			`SELECT * FROM foo LEFT JOIN bar ON 'embedded ''quote'' in string' = foo.b WHERE foo.name = 'String'`,
			"SELECT * FROM foo LEFT JOIN bar ON ? = foo.b WHERE foo.name = ?",
		},
		{
			`SELECT * FROM foo LEFT JOIN bar ON 'embedded \'quote\' in string' = foo.b WHERE foo.name = 'String'`,
			"SELECT * FROM foo LEFT JOIN bar ON ? = foo.b WHERE foo.name = ?",
		},
		{
			"SELECT org_id,metric_key,metric_type,interval FROM metrics_metadata WHERE org_id = ? AND metric_key = ANY(ARRAY[?,?,?,?,?])",
			"SELECT org_id, metric_key, metric_type, interval FROM metrics_metadata WHERE org_id = ? AND metric_key = ANY ( ARRAY [ ? ] )",
		},
		{
			`SELECT wp_woocommerce_order_items.order_id As No_Commande
			FROM  wp_woocommerce_order_items
			LEFT JOIN
				(
					SELECT meta_value As Prenom
					FROM wp_postmeta
					WHERE meta_key = '_shipping_first_name'
				) AS a
			ON wp_woocommerce_order_items.order_id = a.post_id
			WHERE  wp_woocommerce_order_items.order_id =2198`,
			"SELECT wp_woocommerce_order_items.order_id FROM wp_woocommerce_order_items LEFT JOIN ( SELECT meta_value FROM wp_postmeta WHERE meta_key = ? ) ON wp_woocommerce_order_items.order_id = a.post_id WHERE wp_woocommerce_order_items.order_id = ?",
		},
		{
			`SELECT a :: VARCHAR(255) FROM foo WHERE foo.name = 'String'`,
			`SELECT a :: VARCHAR ( ? ) FROM foo WHERE foo.name = ?`,
		},
		{
			"SELECT MIN(`scoped_49a39c4cc9ae4fdda07bcf49e99f8224`.`scoped_8720d2c0e0824ec2910ab9479085839c`) AS `MIN_BECR_DATE_CREATED` FROM (SELECT `49a39c4cc9ae4fdda07bcf49e99f8224`.`submittedOn` AS `scoped_8720d2c0e0824ec2910ab9479085839c`, `49a39c4cc9ae4fdda07bcf49e99f8224`.`domain` AS `scoped_847e4dcfa1c54d72aad6dbeb231c46de`, `49a39c4cc9ae4fdda07bcf49e99f8224`.`eventConsumer` AS `scoped_7b2f7b8da15646d1b75aa03901460eb2`, `49a39c4cc9ae4fdda07bcf49e99f8224`.`eventType` AS `scoped_77a1b9308b384a9391b69d24335ba058` FROM (`SorDesignTime`.`businessEventConsumerRegistry_947a74dad4b64be9847d67f466d26f5e` AS `49a39c4cc9ae4fdda07bcf49e99f8224`) WHERE (`49a39c4cc9ae4fdda07bcf49e99f8224`.`systemData.ClientID`) = ('35c1ccc0-a83c-4812-a189-895e9d4dd223')) AS `scoped_49a39c4cc9ae4fdda07bcf49e99f8224` WHERE ((`scoped_49a39c4cc9ae4fdda07bcf49e99f8224`.`scoped_847e4dcfa1c54d72aad6dbeb231c46de`) = ('Benefits') AND ((`scoped_49a39c4cc9ae4fdda07bcf49e99f8224`.`scoped_7b2f7b8da15646d1b75aa03901460eb2`) = ('benefits') AND (`scoped_49a39c4cc9ae4fdda07bcf49e99f8224`.`scoped_77a1b9308b384a9391b69d24335ba058`) = ('DMXSync'))); ",
			"SELECT MIN ( scoped_49a39c4cc9ae4fdda07bcf49e99f8224 . scoped_8720d2c0e0824ec2910ab9479085839c ) FROM ( SELECT 49a39c4cc9ae4fdda07bcf49e99f8224 . submittedOn, 49a39c4cc9ae4fdda07bcf49e99f8224 . domain, 49a39c4cc9ae4fdda07bcf49e99f8224 . eventConsumer, 49a39c4cc9ae4fdda07bcf49e99f8224 . eventType FROM ( SorDesignTime . businessEventConsumerRegistry_947a74dad4b64be9847d67f466d26f5e ) WHERE ( 49a39c4cc9ae4fdda07bcf49e99f8224 . systemData.ClientID ) = ( ? ) ) WHERE ( ( scoped_49a39c4cc9ae4fdda07bcf49e99f8224 . scoped_847e4dcfa1c54d72aad6dbeb231c46de ) = ( ? ) AND ( ( scoped_49a39c4cc9ae4fdda07bcf49e99f8224 . scoped_7b2f7b8da15646d1b75aa03901460eb2 ) = ( ? ) AND ( scoped_49a39c4cc9ae4fdda07bcf49e99f8224 . scoped_77a1b9308b384a9391b69d24335ba058 ) = ( ? ) ) )",
		},
		{
			"{call px_cu_se_security_pg.sps_get_my_accounts_count(?, ?, ?, ?)}",
			"{ call px_cu_se_security_pg.sps_get_my_accounts_count ( ? ) }",
		},
		{
			`{call px_cu_se_security_pg.sps_get_my_accounts_count(1, 2, 'one', 'two')};`,
			"{ call px_cu_se_security_pg.sps_get_my_accounts_count ( ? ) }",
		},
		{
			`{call curly_fun('{{', '}}', '}', '}')};`,
			"{ call curly_fun ( ? ) }",
		},
		{
			`SELECT id, name FROM emp WHERE name LIKE {fn UCASE('Smith')}`,
			`SELECT id, name FROM emp WHERE name LIKE ?`,
		},
		{
			`DROP TABLE IF EXISTS django_site;
DROP TABLE IF EXISTS knowledgebase_article;

CREATE TABLE django_site (
    id integer PRIMARY KEY,
    domain character varying(100) NOT NULL,
    name character varying(50) NOT NULL,
    uuid uuid NOT NULL,
    disabled boolean DEFAULT false NOT NULL
);

CREATE TABLE knowledgebase_article (
    id integer PRIMARY KEY,
    title character varying(255) NOT NULL,
    site_id integer NOT NULL,
    CONSTRAINT knowledgebase_article_site_id_fkey FOREIGN KEY (site_id) REFERENCES django_site(id)
);

INSERT INTO django_site(id, domain, name, uuid, disabled) VALUES (1, 'foo.domain', 'Foo', 'cb4776c1-edf3-4041-96a8-e152f5ae0f91', false);
INSERT INTO knowledgebase_article(id, title, site_id) VALUES(1, 'title', 1);`,
			`DROP TABLE IF EXISTS django_site DROP TABLE IF EXISTS knowledgebase_article CREATE TABLE django_site ( id integer PRIMARY KEY, domain character varying ( ? ) NOT ? name character varying ( ? ) NOT ? uuid uuid NOT ? disabled boolean DEFAULT ? NOT ? ) CREATE TABLE knowledgebase_article ( id integer PRIMARY KEY, title character varying ( ? ) NOT ? site_id integer NOT ? CONSTRAINT knowledgebase_article_site_id_fkey FOREIGN KEY ( site_id ) REFERENCES django_site ( id ) ) INSERT INTO django_site ( id, domain, name, uuid, disabled ) VALUES ( ? ) INSERT INTO knowledgebase_article ( id, title, site_id ) VALUES ( ? )`,
		},
		{
			`
SELECT set_config('foo.bar', (SELECT foo.bar FROM sometable WHERE sometable.uuid = %(some_id)s)::text, FALSE);
SELECT
    othertable.id,
    othertable.title
FROM othertable
INNER JOIN sometable ON sometable.id = othertable.site_id
WHERE
    sometable.uuid = %(some_id)s
LIMIT 1
;`,
			`SELECT set_config ( ? ( SELECT foo.bar FROM sometable WHERE sometable.uuid = ? ) :: text, ? ) SELECT othertable.id, othertable.title FROM othertable INNER JOIN sometable ON sometable.id = othertable.site_id WHERE sometable.uuid = ? LIMIT ?`,
		},
		{
			`CREATE OR REPLACE FUNCTION pg_temp.sequelize_upsert(OUT created boolean, OUT primary_key text) AS $func$ BEGIN INSERT INTO "school" ("id","organization_id","name","created_at","updated_at") VALUES ('dc4e9444-d7c9-40a9-bcef-68e4cc594e61','ec647f56-f27a-49a1-84af-021ad0a19f21','Test','2021-03-31 16:30:43.915 +00:00','2021-03-31 16:30:43.915 +00:00'); created := true; EXCEPTION WHEN unique_violation THEN UPDATE "school" SET "id"='dc4e9444-d7c9-40a9-bcef-68e4cc594e61',"organization_id"='ec647f56-f27a-49a1-84af-021ad0a19f21',"name"='Test',"updated_at"='2021-03-31 16:30:43.915 +00:00' WHERE ("id" = 'dc4e9444-d7c9-40a9-bcef-68e4cc594e61'); created := false; END; $func$ LANGUAGE plpgsql; SELECT * FROM pg_temp.sequelize_upsert();`,
			`CREATE OR REPLACE FUNCTION pg_temp.sequelize_upsert ( OUT created boolean, OUT primary_key text ) LANGUAGE plpgsql SELECT * FROM pg_temp.sequelize_upsert ( )`,
		},
		{
			`INSERT INTO table (field1, field2) VALUES (1, $$someone's string123$with other things$$)`,
			`INSERT INTO table ( field1, field2 ) VALUES ( ? )`,
		},
		{
			`INSERT INTO table (field1) VALUES ($some tag$this text confuses$some other text$some ta not quite$some tag$)`,
			`INSERT INTO table ( field1 ) VALUES ( ? )`,
		},
		{
			`INSERT INTO table (field1) VALUES ($tag$random \wqejks "sadads' text$tag$)`,
			`INSERT INTO table ( field1 ) VALUES ( ? )`,
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			s := SQLSpan(c.query)
			NewObfuscator(nil).Obfuscate(s)
			assert.Equal(t, c.expected, s.Resource)
		})
	}
}

func TestSQLTokenizerIgnoreEscapeFalse(t *testing.T) {
	cases := []sqlTokenizerTestCase{
		{
			`'Simple string'`,
			"Simple string",
			String,
		},
		{
			`'String with backslash at end \'`,
			"String with backslash at end '",
			LexError,
		},
		{
			`'String with backslash \ in the middle'`,
			"String with backslash  in the middle",
			String,
		},
		{
			`'String with double-backslash at end \\'`,
			"String with double-backslash at end \\",
			String,
		},
		{
			`'String with double-backslash \\ in the middle'`,
			"String with double-backslash \\ in the middle",
			String,
		},
		{
			`'String with backslash-escaped quote at end \''`,
			"String with backslash-escaped quote at end '",
			String,
		},
		{
			`'String with backslash-escaped quote \' in middle'`,
			"String with backslash-escaped quote ' in middle",
			String,
		},
		{
			`'String with backslash-escaped embedded string \'foo\' in the middle'`,
			"String with backslash-escaped embedded string 'foo' in the middle",
			String,
		},
		{
			`'String with backslash-escaped embedded string at end \'foo\''`,
			"String with backslash-escaped embedded string at end 'foo'",
			String,
		},
		{
			`'String with double-backslash-escaped embedded string at the end \\'foo\\''`,
			"String with double-backslash-escaped embedded string at the end \\",
			String,
		},
		{
			`'String with double-backslash-escaped embedded string \\'foo\\' in the middle'`,
			"String with double-backslash-escaped embedded string \\",
			String,
		},
		{
			`'String with backslash-escaped embedded string \'foo\' in the middle followed by one at the end \'`,
			"String with backslash-escaped embedded string 'foo' in the middle followed by one at the end '",
			LexError,
		},
		{
			`'String with embedded string at end ''foo'''`,
			"String with embedded string at end 'foo'",
			String,
		},
		{
			`'String with embedded string ''foo'' in the middle'`,
			"String with embedded string 'foo' in the middle",
			String,
		},
		{
			`'String with tab at end	'`,
			"String with tab at end\t",
			String,
		},
		{
			`'String with tab	in the middle'`,
			"String with tab\tin the middle",
			String,
		},
		{
			`'String with newline at the end
'`,
			"String with newline at the end\n",
			String,
		},
		{
			`'String with newline
in the middle'`,
			"String with newline\nin the middle",
			String,
		},
		{
			`'Simple string missing closing quote`,
			"Simple string missing closing quote",
			LexError,
		},
		{
			`'String missing closing quote with backslash at end \`,
			"String missing closing quote with backslash at end ",
			LexError,
		},
		{
			`'String with backslash \ in the middle missing closing quote`,
			"String with backslash  in the middle missing closing quote",
			LexError,
		},
		{
			`::`,
			`::`,
			ColonCast,
		},
		// The following case will treat the final quote as unescaped
		{
			`'String missing closing quote with backslash-escaped quote at end \'`,
			"String missing closing quote with backslash-escaped quote at end '",
			LexError,
		},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("tokenize_%s", c.str), func(t *testing.T) {
			tokenizer := NewSQLTokenizer(c.str, false)
			kind, buffer := tokenizer.Scan()
			assert.Equal(t, c.expectedKind, kind)
			assert.Equal(t, c.expected, string(buffer))
		})
	}
}

func TestSQLTokenizerIgnoreEscapeTrue(t *testing.T) {
	cases := []sqlTokenizerTestCase{
		{
			`'Simple string'`,
			"Simple string",
			String,
		},
		{
			`'String with backslash at end \'`,
			"String with backslash at end \\",
			String,
		},
		{
			`'String with backslash \ in the middle'`,
			"String with backslash \\ in the middle",
			String,
		},
		{
			`'String with double-backslash at end \\'`,
			"String with double-backslash at end \\\\",
			String,
		},
		{
			`'String with double-backslash \\ in the middle'`,
			"String with double-backslash \\\\ in the middle",
			String,
		},
		// The following case will treat backslash as literal and double single quote as a single quote
		// thus missing the final single quote
		{
			`'String with backslash-escaped quote at end \''`,
			"String with backslash-escaped quote at end \\'",
			LexError,
		},
		{
			`'String with backslash-escaped quote \' in middle'`,
			"String with backslash-escaped quote \\",
			String,
		},
		{
			`'String with backslash-escaped embedded string at the end \'foo\''`,
			"String with backslash-escaped embedded string at the end \\",
			String,
		},
		{
			`'String with backslash-escaped embedded string \'foo\' in the middle'`,
			"String with backslash-escaped embedded string \\",
			String,
		},
		{
			`'String with double-backslash-escaped embedded string at end \\'foo\\''`,
			"String with double-backslash-escaped embedded string at end \\\\",
			String,
		},
		{
			`'String with double-backslash-escaped embedded string \\'foo\\' in the middle'`,
			"String with double-backslash-escaped embedded string \\\\",
			String,
		},
		{
			`'String with backslash-escaped embedded string \'foo\' in the middle followed by one at the end \'`,
			"String with backslash-escaped embedded string \\",
			String,
		},
		{
			`'String with embedded string at end ''foo'''`,
			"String with embedded string at end 'foo'",
			String,
		},
		{
			`'String with embedded string ''foo'' in the middle'`,
			"String with embedded string 'foo' in the middle",
			String,
		},
		{
			`'String with tab at end	'`,
			"String with tab at end\t",
			String,
		},
		{
			`'String with tab	in the middle'`,
			"String with tab\tin the middle",
			String,
		},
		{
			`'String with newline at the end
'`,
			"String with newline at the end\n",
			String,
		},
		{
			`'String with newline
in the middle'`,
			"String with newline\nin the middle",
			String,
		},
		{
			`'Simple string missing closing quote`,
			"Simple string missing closing quote",
			LexError,
		},
		{
			`'String missing closing quote with backslash at end \`,
			"String missing closing quote with backslash at end \\",
			LexError,
		},
		{
			`'String with backslash \ in the middle missing closing quote`,
			"String with backslash \\ in the middle missing closing quote",
			LexError,
		},
		// The following case will treat the final quote as unescaped
		{
			`'String missing closing quote with backslash-escaped quote at end \'`,
			"String missing closing quote with backslash-escaped quote at end \\",
			String,
		},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("tokenize_%s", c.str), func(t *testing.T) {
			tokenizer := NewSQLTokenizer(c.str, true)
			tokenizer.literalEscapes = true
			kind, buffer := tokenizer.Scan()
			assert.Equal(t, c.expectedKind, kind)
			assert.Equal(t, c.expected, string(buffer))
		})
	}
}

func TestMultipleProcess(t *testing.T) {
	assert := assert.New(t)

	testCases := []struct {
		query    string
		expected string
	}{
		{
			"SELECT clients.* FROM clients INNER JOIN posts ON posts.author_id = author.id AND posts.published = 't'",
			"SELECT clients.* FROM clients INNER JOIN posts ON posts.author_id = author.id AND posts.published = ?",
		},
		{
			"SELECT articles.* FROM articles WHERE articles.id IN (1, 3, 5)",
			"SELECT articles.* FROM articles WHERE articles.id IN ( ? )",
		},
		{
			`SELECT id FROM jq_jobs
WHERE
schedulable_at <= 1555367948 AND
queue_name = 'order_jobs' AND
status = 1 AND
id % 8 = 3
ORDER BY
schedulable_at
LIMIT 1000`,
			"SELECT id FROM jq_jobs WHERE schedulable_at <= ? AND queue_name = ? AND status = ? AND id % ? = ? ORDER BY schedulable_at LIMIT ?",
		},
	}

	// The consumer is the same between executions
	for _, tc := range testCases {
		oq, err := NewObfuscator(nil).ObfuscateSQLString(tc.query)
		assert.Nil(err)
		assert.Equal(tc.expected, oq.Query)
	}
}

func TestConsumerError(t *testing.T) {
	assert := assert.New(t)

	// Malformed SQL is not accepted and the outer component knows
	// what to do with malformed SQL
	input := "SELECT * FROM users WHERE users.id = '1 AND users.name = 'dog'"

	_, err := NewObfuscator(nil).ObfuscateSQLString(input)
	assert.NotNil(err)
}

func TestSQLErrors(t *testing.T) {
	cases := []sqlTestCase{
		{
			"",
			"result is empty",
		},

		{
			"SELECT a FROM b WHERE a.x !* 2",
			`at position 27: expected "=" after "!", got "*" (42)`,
		},

		{
			"SELECT 🥒",
			`at position 11: unexpected byte 129362`,
		},
		{
			"SELECT name, `age}` FROM profile",
			`at position 17: literal identifiers must end in "` + "`" + `", got "}" (125)`,
		},

		{
			"SELECT %(asd)| FROM profile",
			`at position 13: invalid character after variable identifier: "|" (124)`,
		},

		{
			"USING $A FROM users",
			`at position 20: unexpected EOF in string`,
		},

		{
			"USING $09 SELECT",
			`at position 9: invalid number`,
		},

		{
			"INSERT VALUES (1, 2) INTO {ABC",
			`at position 30: unexpected EOF in escape sequence`,
		},

		{
			"SELECT one, :2two FROM profile",
			`at position 13: bind variables should start with letters, got "2" (50)`,
		},

		{
			"SELECT age FROM profile WHERE name='John \\",
			`at position 43: unexpected EOF in string`,
		},

		{
			"SELECT age FROM profile WHERE name='John",
			`at position 41: unexpected EOF in string`,
		},

		{
			"/* abcd",
			`at position 7: unexpected EOF in comment`,
		},

		// using mixed cases of backslash escaping the single quote
		{
			"SELECT age FROM profile WHERE name='John\\' and place='John\\'s House'",
			`at position 59: unexpected byte 92`,
		},

		{
			"SELECT age FROM profile WHERE place='John\\'s House' and name='John\\'",
			`at position 69: unexpected EOF in string`,
		},

		{
			" \x80",
			"at position 1: invalid UTF-8 encoding beginning with 0x80",
		},

		{
			"\x3a\xdb",
			"at position 1: invalid UTF-8 encoding beginning with 0xdb",
		},

		{
			"select * from profile where age = \"\x3a\xeb\"",
			"at position 36: invalid UTF-8 encoding beginning with 0xeb",
		},
	}
	for _, tc := range cases {
		t.Run("", func(t *testing.T) {
			_, err := NewObfuscator(nil).ObfuscateSQLString(tc.query)
			assert.Error(t, err)
			assert.Equal(t, tc.expected, err.Error())
		})
	}
}

func TestLiteralEscapesUpdates(t *testing.T) {
	for _, c := range []struct {
		initial bool
		query   string
		err     error
		want    bool
	}{
		{
			false,
			`SELECT * FROM foo WHERE field1 = 'value1' AND field2 = 'value2'`,
			nil,
			false,
		},
		{
			true,
			`SELECT * FROM foo WHERE field1 = 'value1' AND field2 = 'value2'`,
			nil,
			true,
		},
		{
			false,
			`SELECT * FROM foo WHERE name = 'backslash\' AND id ='1234'`,
			nil,
			true,
		},
		{
			true,
			`SELECT * FROM foo WHERE name = 'embedded \'string\' in quotes' AND id ='1234'`,
			nil,
			false,
		},
		{
			false,
			`SELECT age FROM profile WHERE name='John\' and place='John\'s House'`,
			errors.New("at position 59: unexpected byte 92"),
			false,
		},
		{
			true,
			`SELECT age FROM profile WHERE name='John\' and place='John\'s House'`,
			errors.New("at position 69: unexpected EOF in string"),
			true,
		},
	} {
		t.Run("", func(t *testing.T) {
			o := NewObfuscator(nil)
			o.SetSQLLiteralEscapes(c.initial)
			_, err := o.ObfuscateSQLString(c.query)
			if c.err != nil {
				assert.Equal(t, c.err, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, c.want, o.SQLLiteralEscapes(), "Unexpected final value of SQLLiteralEscapes")
		})
	}
}

// LargeQuery is sourced from https://stackoverflow.com/questions/12607667/issues-with-a-very-large-sql-query/12711494
var LargeQuery = `SELECT '%c%' as Chapter,
(SELECT count(ticket.id) AS Matches FROM engine.ticket INNER JOIN engine.ticket_custom ON ticket.id = ticket_custom.ticket
WHERE ticket_custom.name='chapter' AND ticket_custom.value LIKE '%c%' AND type='New material' AND milestone='1.1.12' AND component NOT LIKE 'internal_engine' AND ticket.status IN ('new','assigned') ) AS 'New',
(SELECT count(ticket.id) AS Matches FROM engine.ticket INNER JOIN engine.ticket_custom ON ticket.id = ticket_custom.ticket
WHERE ticket_custom.name='chapter' AND ticket_custom.value LIKE '%c%' AND type='New material' AND milestone='1.1.12' AND component NOT LIKE 'internal_engine' AND ticket.status='document_interface' ) AS 'Document\
 Interface',
(SELECT count(ticket.id) AS Matches FROM engine.ticket INNER JOIN engine.ticket_custom ON ticket.id = ticket_custom.ticket
WHERE ticket_custom.name='chapter' AND ticket_custom.value LIKE '%c%' AND type='New material' AND milestone='1.1.12' AND component NOT LIKE 'internal_engine' AND ticket.status='interface_development' ) AS 'Inter\
face Development',
(SELECT count(ticket.id) AS Matches FROM engine.ticket INNER JOIN engine.ticket_custom ON ticket.id = ticket_custom.ticket
WHERE ticket_custom.name='chapter' AND ticket_custom.value LIKE '%c%' AND type='New material' AND milestone='1.1.12' AND component NOT LIKE 'internal_engine' AND ticket.status='interface_check' ) AS 'Interface C\
heck',
(SELECT count(ticket.id) AS Matches FROM engine.ticket INNER JOIN engine.ticket_custom ON ticket.id = ticket_custom.ticket
WHERE ticket_custom.name='chapter' AND ticket_custom.value LIKE '%c%' AND type='New material' AND milestone='1.1.12' AND component NOT LIKE 'internal_engine' AND ticket.status='document_routine' ) AS 'Document R\
outine',
(SELECT count(ticket.id) AS Matches FROM engine.ticket INNER JOIN engine.ticket_custom ON ticket.id = ticket_custom.ticket
WHERE ticket_custom.name='chapter' AND ticket_custom.value LIKE '%c%' AND type='New material' AND milestone='1.1.12' AND component NOT LIKE 'internal_engine' AND ticket.status='full_development' ) AS 'Full Devel\
opment',
(SELECT count(ticket.id) AS Matches FROM engine.ticket INNER JOIN engine.ticket_custom ON ticket.id = ticket_custom.ticket
WHERE ticket_custom.name='chapter' AND ticket_custom.value LIKE '%c%' AND type='New material' AND milestone='1.1.12' AND component NOT LIKE 'internal_engine' AND ticket.status='peer_review_1' ) AS 'Peer Review O\
ne',
(SELECT count(ticket.id) AS Matches FROM engine.ticket INNER JOIN engine.ticket_custom ON ticket.id = ticket_custom.ticket
WHERE ticket_custom.name='chapter' AND ticket_custom.value LIKE '%c%'AND type='New material' AND milestone='1.1.12' AND component NOT LIKE 'internal_engine' AND ticket.status='peer_review_2' ) AS 'Peer Review Tw\
o',
(SELECT count(ticket.id) AS Matches FROM engine.ticket INNER JOIN engine.ticket_custom ON ticket.id = ticket_custom.ticket
WHERE ticket_custom.name='chapter' AND ticket_custom.value LIKE '%c%' AND type='New material' AND milestone='1.1.12' AND component NOT LIKE 'internal_engine' AND ticket.status='qa' ) AS 'QA',
(SELECT count(ticket.id) AS Matches FROM engine.ticket INNER JOIN engine.ticket_custom ON ticket.id = ticket_custom.ticket
WHERE ticket_custom.name='chapter' AND ticket_custom.value LIKE '%c%'AND type='New material' AND milestone='1.1.12' AND component NOT LIKE 'internal_engine' AND ticket.status='closed' ) AS 'Closed',
count(id) AS Total,
ticket.id AS _id
FROM engine.ticket
INNER JOIN engine.ticket_custom ON ticket.id = ticket_custom.ticket
WHERE ticket_custom.name='chapter' AND ticket_custom.value LIKE '%c%' AND type='New material' AND milestone='1.1.12' AND component NOT LIKE 'internal_engine'`

// query3 is sourced from https://www.ibm.com/support/knowledgecenter/SSCRJT_6.0.0/com.ibm.swg.im.bigsql.doc/doc/tut_bsql_uc_complex_query.html
var ComplexQuery = `WITH
 sales AS
 (SELECT sf.*
  FROM gosalesdw.sls_order_method_dim AS md,
       gosalesdw.sls_product_dim AS pd,
       gosalesdw.emp_employee_dim AS ed,
       gosalesdw.sls_sales_fact AS sf
  WHERE pd.product_key = sf.product_key
    AND pd.product_number > 10000
    AND pd.base_product_key > 30
    AND md.order_method_key = sf.order_method_key
    AND md.order_method_code > 5
    AND ed.employee_key = sf.employee_key
    AND ed.manager_code1 > 20),
 inventory AS
 (SELECT if.*
  FROM gosalesdw.go_branch_dim AS bd,
    gosalesdw.dist_inventory_fact AS if
  WHERE if.branch_key = bd.branch_key
    AND bd.branch_code > 20)
SELECT sales.product_key AS PROD_KEY,
 SUM(CAST (inventory.quantity_shipped AS BIGINT)) AS INV_SHIPPED,
 SUM(CAST (sales.quantity AS BIGINT)) AS PROD_QUANTITY,
 RANK() OVER ( ORDER BY SUM(CAST (sales.quantity AS BIGINT)) DESC) AS PROD_RANK
FROM sales, inventory
 WHERE sales.product_key = inventory.product_key
GROUP BY sales.product_key;
`

// Benchmark the Tokenizer using a SQL statement
func BenchmarkObfuscateSQLString(b *testing.B) {
	benchmarks := []struct {
		name  string
		query string
	}{
		{"Escaping", `INSERT INTO delayed_jobs (attempts, created_at, failed_at, handler, last_error, locked_at, locked_by, priority, queue, run_at, updated_at) VALUES (0, '2016-12-04 17:09:59', NULL, '--- !ruby/object:Delayed::PerformableMethod\nobject: !ruby/object:Item\n  store:\n  - a simple string\n  - an \'escaped \' string\n  - another \'escaped\' string\n  - 42\n  string: a string with many \\\\\'escapes\\\\\'\nmethod_name: :show_store\nargs: []\n', NULL, NULL, NULL, 0, NULL, '2016-12-04 17:09:59', '2016-12-04 17:09:59')`},
		{"Grouping", `INSERT INTO delayed_jobs (created_at, failed_at, handler) VALUES (0, '2016-12-04 17:09:59', NULL), (0, '2016-12-04 17:09:59', NULL), (0, '2016-12-04 17:09:59', NULL), (0, '2016-12-04 17:09:59', NULL)`},
		{"Large", LargeQuery},
		{"Complex", ComplexQuery},
	}
	obf := NewObfuscator(nil)
	for _, bm := range benchmarks {
		b.Run(bm.name+"/"+strconv.Itoa(len(bm.query)), func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := obf.ObfuscateSQLString(bm.query)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}

	b.Run("random", func(b *testing.B) {
		b.ReportAllocs()
		var j uint64
		for i := 0; i < b.N; i++ {
			_, err := obf.ObfuscateSQLString(fmt.Sprintf("SELECT * FROM users WHERE id=%d", atomic.AddUint64(&j, 1)))
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkQueryCacheTippingPoint is meant to help evaluate the minimum cache hit rate needed for the
// query cache to become performance beneficial.
//
// The first test in each suite (called "off") is the comparison point without a cache. The tipping
// point is the hit rate at which the results are better than "off", with cache.
func BenchmarkQueryCacheTippingPoint(b *testing.B) {
	queries := 1000

	bench1KQueries := func(
		fn func(*Obfuscator, string) (*ObfuscatedQuery, error), // obfuscating function
		hitrate float64, // desired cache hit rate
		queryfmt string, // actual query (passed to fmt.Sprintf)
	) func(*testing.B) {
		if hitrate < 0 || hitrate > 1 {
			b.Fatalf("invalid hit rate %.2f", hitrate)
		}
		return func(b *testing.B) {
			o := NewObfuscator(nil)
			hitcount := int(float64(queries) * hitrate)
			var idx uint64
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for n := 0; n < hitcount; n++ {
					if _, err := fn(o, fmt.Sprintf(queryfmt, -1)); err != nil {
						b.Fatal(err)
					}
				}
				for n := 0; n < queries-hitcount; n++ {
					if _, err := fn(o, fmt.Sprintf(queryfmt, atomic.AddUint64(&idx, 1))); err != nil {
						b.Fatal(err)
					}
				}
			}
		}
	}

	for name, queryfmt := range map[string]string{
		"shorter":     `SELECT * FROM users WHERE id=%d`,
		"medium":      `INSERT INTO delayed_jobs (created_at, failed_at, handler) VALUES (%d, '2016-12-04 17:09:5912', NULL)`,
		"medium-long": `INSERT INTO delayed_jobs (created_at, failed_at, handler) VALUES (%d, '2016-12-04 17:09:59', NULL), (0, '2016-12-04 17:09:59', NULL), (0, '2016-12-04 17:09:59', NULL), (0, '2016-12-04 17:09:59', NULL)`,
		"long":        "SELECT\r\n\t                CodiFormacio\r\n\t                ,DataInici\r\n\t                ,DataFi\r\n\t                ,Tipo\r\n\t                ,CodiTecnicFormador\r\n\t                ,p.nombre AS TutorNombre\r\n\t                ,p.mail AS TutorMail\r\n\t                ,Sessions.Direccio\r\n\t                ,Sessions.NomEmpresa\r\n\t                ,Sessions.Telefon\r\n FROM\r\n\r\n\t when ModalitatSessio = '2' then 'Presencial'--Practica\r\n\t  when ModalitatSessio = '3' then 'Online'--Tutoria\r\n  when ModalitatSessio = '4' then 'Presencial'--Examen\r\n\t  ELSE 'Presencial'\r\n\t end as Tipo\r\n\t   ,ModalitatSessio\r\n\t ,DataInici\r\n\t  ,DataFi\r\n    ,CASE\r\n\t                   WHEn EsAltres = 1 then FormacioLlocImparticioDescripcio\r\n\t else Adreca + ' - ' + CodiPostal + ' ' + Poblacio\r\n\t                end as Direccio\r\n\t\r\n                FROM Consultas.dbo.View_AsActiva__FormacioSessions_InfoLlocImparticio) AS Sessions\r\n WHERE Sessions.CodiFormacio = '%d'",
		"longer":      "SELECT\r\n\t                CodiFormacio\r\n\t                ,DataInici\r\n\t                ,DataFi\r\n\t                ,Tipo\r\n\t                ,CodiTecnicFormador\r\n\t                ,p.nombre AS TutorNombre\r\n\t                ,p.mail AS TutorMail\r\n\t                ,Sessions.Direccio\r\n\t                ,Sessions.NomEmpresa\r\n\t                ,Sessions.Telefon\r\n                FROM\r\n                ----------------------------\r\n                (SELECT\r\n\t                CodiFormacio\r\n\t                ,case\r\n\t                   when ModalitatSessio = '1' then 'Presencial'--Teoria\r\n\t                   when ModalitatSessio = '2' then 'Presencial'--Practica\r\n\t                   when ModalitatSessio = '3' then 'Online'--Tutoria\r\n                       when ModalitatSessio = '4' then 'Presencial'--Examen\r\n\t                   ELSE 'Presencial'\r\n\t                end as Tipo\r\n\t                ,ModalitatSessio\r\n\t                ,DataInici\r\n\t                ,DataFi\r\n                     ,NomEmpresa\r\n\t                ,Telefon\r\n\t                ,CodiTecnicFormador\r\n\t                ,CASE\r\n\t                   WHEn EsAltres = 1 then FormacioLlocImparticioDescripcio\r\n\t                   else Adreca + ' - ' + CodiPostal + ' ' + Poblacio\r\n\t                end as Direccio\r\n\t\r\n                FROM Consultas.dbo.View_AsActiva__FormacioSessions_InfoLlocImparticio) AS Sessions\r\n                ----------------------------------------\r\n                LEFT JOIN Consultas.dbo.View_AsActiva_Operari AS o\r\n\t                ON o.CodiOperari = Sessions.CodiTecnicFormador\r\n                LEFT JOIN MainAPP.dbo.persona AS p\r\n\t                ON 'preven\\' + o.codioperari = p.codi\r\n                WHERE Sessions.CodiFormacio = '%d'",
		"xlong":       "select top ? percent IdTrebEmpresa, CodCli, NOMEMP, Baixa, CASE WHEN IdCentreTreball IS ? THEN ? ELSE CONVERT ( VARCHAR ( ? ) IdCentreTreball ) END, CASE WHEN NOMESTAB IS ? THEN ? ELSE NOMESTAB END, TIPUS, CASE WHEN IdLloc IS ? THEN ? ELSE CONVERT ( VARCHAR ( ? ) IdLloc ) END, CASE WHEN NomLlocComplert IS ? THEN ? ELSE NomLlocComplert END, CASE WHEN DesLloc IS ? THEN ? ELSE DesLloc END, IdLlocTreballUnic From ( SELECT ?, dbo.Treb_Empresa.IdTrebEmpresa, dbo.Treb_Empresa.IdTreballador, dbo.Treb_Empresa.CodCli, dbo.Clients.NOMEMP, dbo.Treb_Empresa.Baixa, dbo.Treb_Empresa.IdCentreTreball, dbo.Cli_Establiments.NOMESTAB, ?, ?, dbo.Treb_Empresa.DataInici, dbo.Treb_Empresa.DataFi, CASE WHEN dbo.Treb_Empresa.DesLloc IS ? THEN ? ELSE dbo.Treb_Empresa.DesLloc END DesLloc, dbo.Treb_Empresa.IdLlocTreballUnic FROM dbo.Clients WITH ( NOLOCK ) INNER JOIN dbo.Treb_Empresa WITH ( NOLOCK ) ON dbo.Clients.CODCLI = dbo.Treb_Empresa.CodCli LEFT OUTER JOIN dbo.Cli_Establiments WITH ( NOLOCK ) ON dbo.Cli_Establiments.Id_ESTAB_CLI = dbo.Treb_Empresa.IdCentreTreball AND dbo.Cli_Establiments.CODCLI = dbo.Treb_Empresa.CodCli WHERE dbo.Treb_Empresa.IdTreballador = ? AND Treb_Empresa.IdTecEIRLLlocTreball IS ? AND IdMedEIRLLlocTreball IS ? AND IdLlocTreballTemporal IS ? UNION ALL SELECT ?, dbo.Treb_Empresa.IdTrebEmpresa, dbo.Treb_Empresa.IdTreballador, dbo.Treb_Empresa.CodCli, dbo.Clients.NOMEMP, dbo.Treb_Empresa.Baixa, dbo.Treb_Empresa.IdCentreTreball, dbo.Cli_Establiments.NOMESTAB, dbo.Treb_Empresa.IdTecEIRLLlocTreball, dbo.fn_NomLlocComposat ( dbo.Treb_Empresa.IdTecEIRLLlocTreball ), dbo.Treb_Empresa.DataInici, dbo.Treb_Empresa.DataFi, CASE WHEN dbo.Treb_Empresa.DesLloc IS ? THEN ? ELSE dbo.Treb_Empresa.DesLloc END DesLloc, dbo.Treb_Empresa.IdLlocTreballUnic FROM dbo.Clients WITH ( NOLOCK ) INNER JOIN dbo.Treb_Empresa WITH ( NOLOCK ) ON dbo.Clients.CODCLI = dbo.Treb_Empresa.CodCli LEFT OUTER JOIN dbo.Cli_Establiments WITH ( NOLOCK ) ON dbo.Cli_Establiments.Id_ESTAB_CLI = dbo.Treb_Empresa.IdCentreTreball AND dbo.Cli_Establiments.CODCLI = dbo.Treb_Empresa.CodCli WHERE ( dbo.Treb_Empresa.IdTreballador = ? ) AND ( NOT ( dbo.Treb_Empresa.IdTecEIRLLlocTreball IS ? ) ) UNION ALL SELECT ?, dbo.Treb_Empresa.IdTrebEmpresa, dbo.Treb_Empresa.IdTreballador, dbo.Treb_Empresa.CodCli, dbo.Clients.NOMEMP, dbo.Treb_Empresa.Baixa, dbo.Treb_Empresa.IdCentreTreball, dbo.Cli_Establiments.NOMESTAB, dbo.Treb_Empresa.IdMedEIRLLlocTreball, dbo.fn_NomMedEIRLLlocComposat ( dbo.Treb_Empresa.IdMedEIRLLlocTreball ), dbo.Treb_Empresa.DataInici, dbo.Treb_Empresa.DataFi, CASE WHEN dbo.Treb_Empresa.DesLloc IS ? THEN ? ELSE dbo.Treb_Empresa.DesLloc END DesLloc, dbo.Treb_Empresa.IdLlocTreballUnic FROM dbo.Clients WITH ( NOLOCK ) INNER JOIN dbo.Treb_Empresa WITH ( NOLOCK ) ON dbo.Clients.CODCLI = dbo.Treb_Empresa.CodCli LEFT OUTER JOIN dbo.Cli_Establiments WITH ( NOLOCK ) ON dbo.Cli_Establiments.Id_ESTAB_CLI = dbo.Treb_Empresa.IdCentreTreball AND dbo.Cli_Establiments.CODCLI = dbo.Treb_Empresa.CodCli WHERE ( dbo.Treb_Empresa.IdTreballador = ? ) AND ( Treb_Empresa.IdTecEIRLLlocTreball IS ? ) AND ( NOT ( dbo.Treb_Empresa.IdMedEIRLLlocTreball IS ? ) ) UNION ALL SELECT ?, dbo.Treb_Empresa.IdTrebEmpresa, dbo.Treb_Empresa.IdTreballador, dbo.Treb_Empresa.CodCli, dbo.Clients.NOMEMP, dbo.Treb_Empresa.Baixa, dbo.Treb_Empresa.IdCentreTreball, dbo.Cli_Establiments.NOMESTAB, dbo.Treb_Empresa.IdLlocTreballTemporal, dbo.Lloc_Treball_Temporal.NomLlocTreball, dbo.Treb_Empresa.DataInici, dbo.Treb_Empresa.DataFi, CASE WHEN dbo.Treb_Empresa.DesLloc IS ? THEN ? ELSE dbo.Treb_Empresa.DesLloc END DesLloc, dbo.Treb_Empresa.IdLlocTreballUnic FROM dbo.Clients WITH ( NOLOCK ) INNER JOIN dbo.Treb_Empresa WITH ( NOLOCK ) ON dbo.Clients.CODCLI = dbo.Treb_Empresa.CodCli INNER JOIN dbo.Lloc_Treball_Temporal WITH ( NOLOCK ) ON dbo.Treb_Empresa.IdLlocTreballTemporal = dbo.Lloc_Treball_Temporal.IdLlocTreballTemporal LEFT OUTER JOIN dbo.Cli_Establiments WITH ( NOLOCK ) ON dbo.Cli_Establiments.Id_ESTAB_CLI = dbo.Treb_Empresa.IdCentreTreball AND dbo.Cli_Establiments.CODCLI = dbo.Treb_Empresa.CodCli WHERE dbo.Treb_Empresa.IdTreballador = ? AND Treb_Empresa.IdTecEIRLLlocTreball IS ? AND IdMedEIRLLlocTreball IS ? ) Where ? = %d",
	} {
		b.Run(fmt.Sprintf("%s-%d", name, len(queryfmt)), func(b *testing.B) {
			b.Run("off", bench1KQueries((*Obfuscator).obfuscateSQLString, 1, queryfmt))
			b.Run("0%", bench1KQueries((*Obfuscator).ObfuscateSQLString, 0, queryfmt))
			b.Run("1%", bench1KQueries((*Obfuscator).ObfuscateSQLString, 0.01, queryfmt))
			b.Run("5%", bench1KQueries((*Obfuscator).ObfuscateSQLString, 0.05, queryfmt))
			b.Run("10%", bench1KQueries((*Obfuscator).ObfuscateSQLString, 0.1, queryfmt))
			b.Run("20%", bench1KQueries((*Obfuscator).ObfuscateSQLString, 0.2, queryfmt))
			b.Run("30%", bench1KQueries((*Obfuscator).ObfuscateSQLString, 0.3, queryfmt))
			b.Run("50%", bench1KQueries((*Obfuscator).ObfuscateSQLString, 0.5, queryfmt))
			b.Run("70%", bench1KQueries((*Obfuscator).ObfuscateSQLString, 0.7, queryfmt))
			b.Run("100%", bench1KQueries((*Obfuscator).ObfuscateSQLString, 1, queryfmt))
		})
	}
}

func CassSpan(query string) *pb.Span {
	return &pb.Span{
		Resource: query,
		Type:     "cassandra",
		Meta: map[string]string{
			"query": query,
		},
	}
}

func TestCassQuantizer(t *testing.T) {
	assert := assert.New(t)

	queryToExpected := []struct{ in, expected string }{
		// List compacted and replaced
		{
			"select key, status, modified from org_check_run where org_id = %s and check in (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)",
			"select key, status, modified from org_check_run where org_id = ? and check in ( ? )",
		},
		// Some whitespace-y things
		{
			"select key, status, modified from org_check_run where org_id = %s and check in (%s, %s, %s)",
			"select key, status, modified from org_check_run where org_id = ? and check in ( ? )",
		},
		{
			"select key, status, modified from org_check_run where org_id = %s and check in (%s , %s , %s )",
			"select key, status, modified from org_check_run where org_id = ? and check in ( ? )",
		},
		// %s replaced with ? as in sql quantize
		{
			"select key, status, modified from org_check_run where org_id = %s and check = %s",
			"select key, status, modified from org_check_run where org_id = ? and check = ?",
		},
		{
			"select key, status, modified from org_check_run where org_id = %s and check = %s",
			"select key, status, modified from org_check_run where org_id = ? and check = ?",
		},
		{
			"SELECT timestamp, processes FROM process_snapshot.minutely WHERE org_id = ? AND host = ? AND timestamp >= ? AND timestamp <= ?",
			"SELECT timestamp, processes FROM process_snapshot.minutely WHERE org_id = ? AND host = ? AND timestamp >= ? AND timestamp <= ?",
		},
		{
			"SELECT count(*) AS totcount FROM (SELECT \"c1\", \"c2\",\"c3\",\"c4\",\"c5\",\"c6\",\"c7\",\"c8\", \"c9\", \"c10\",\"c11\",\"c12\",\"c13\",\"c14\", \"c15\",\"c16\",\"c17\",\"c18\", \"c19\",\"c20\",\"c21\",\"c22\",\"c23\", \"c24\",\"c25\",\"c26\", \"c27\" FROM (SELECT bar.y AS \"c2\", foo.x AS \"c3\", foo.z AS \"c4\", DECODE(foo.a, NULL,NULL, foo.a ||?|| foo.b) AS \"c5\" , foo.c AS \"c6\", bar.d AS \"c1\", bar.e AS \"c7\", bar.f AS \"c8\", bar.g AS \"c9\", TO_DATE(TO_CHAR(TO_DATE(bar.h,?),?),?) AS \"c10\", TO_DATE(TO_CHAR(TO_DATE(bar.i,?),?),?) AS \"c11\", CASE WHEN DECODE(bar.j, NULL, TRUNC(SYSDATE) - TRUNC(TO_DATE(bar.h,?))) > ? THEN ? WHEN DECODE(bar.j, NULL, TRUNC(SYSDATE) - TRUNC(TO_DATE(bar.h,?))) > ? THEN ? WHEN DECODE(bar.j, NULL, TRUNC(SYSDATE) - TRUNC(TO_DATE(bar.h,?))) > ? THEN ? WHEN DECODE(bar.j, NULL, TRUNC(SYSDATE) - TRUNC(TO_DATE(bar.h,?))) > ? THEN ? WHEN DECODE(bar.j, NULL, TRUNC(SYSDATE) - TRUNC(TO_DATE(bar.h,?))) > ? THEN ? WHEN DECODE(bar.j, NULL, TRUNC(SYSDATE) - TRUNC(TO_DATE(bar.h,?))) > ? THEN ? ELSE NULL END AS \"c12\", DECODE(bar.j, NULL, TRUNC(SYSDATE) - TRUNC(TO_DATE(bar.h,?)),NULL) as \"c13\", bar.k AS \"c14\", bar.l ||?||bar.m AS \"c15\", DECODE(bar.n, NULL, NULL,bar.n ||?||bar.o) AS \"c16\", bar.p AS \"c17\", bar.q AS \"c18\", bar.r AS \"c19\", bar.s AS \"c20\", qux.a AS \"c21\", TO_CHAR(TO_DATE(qux.b,?),?) AS \"c22\", DECODE(qux.l,NULL,NULL, qux.l ||?||qux.m) AS \"c23\", bar.a AS \"c24\", TO_CHAR(TO_DATE(bar.j,?),?) AS \"c25\", DECODE(bar.c , ?,?,?, ?, bar.c ) AS \"c26\", bar.y AS y, bar.d, bar.d AS \"c27\" FROM blort.bar , ( SELECT * FROM (SELECT a,a,l,m,b,c, RANK() OVER (PARTITION BY c ORDER BY b DESC) RNK FROM blort.d WHERE y IN (:p)) WHERE RNK = ?) qux, blort.foo WHERE bar.c = qux.c(+) AND bar.x = foo.x AND bar.y IN (:p) and bar.x IN (:x)) )\nSELECT count(*) AS totcount FROM (SELECT \"c1\", \"c2\",\"c3\",\"c4\",\"c5\",\"c6\",\"c7\",\"c8\", \"c9\", \"c10\",\"c11\",\"c12\",\"c13\",\"c14\", \"c15\",\"c16\",\"c17\",\"c18\", \"c19\",\"c20\",\"c21\",\"c22\",\"c23\", \"c24\",\"c25\",\"c26\", \"c27\" FROM (SELECT bar.y AS \"c2\", foo.x AS \"c3\", foo.z AS \"c4\", DECODE(foo.a, NULL,NULL, foo.a ||?|| foo.b) AS \"c5\" , foo.c AS \"c6\", bar.d AS \"c1\", bar.e AS \"c7\", bar.f AS \"c8\", bar.g AS \"c9\", TO_DATE(TO_CHAR(TO_DATE(bar.h,?),?),?) AS \"c10\", TO_DATE(TO_CHAR(TO_DATE(bar.i,?),?),?) AS \"c11\", CASE WHEN DECODE(bar.j, NULL, TRUNC(SYSDATE) - TRUNC(TO_DATE(bar.h,?))) > ? THEN ? WHEN DECODE(bar.j, NULL, TRUNC(SYSDATE) - TRUNC(TO_DATE(bar.h,?))) > ? THEN ? WHEN DECODE(bar.j, NULL, TRUNC(SYSDATE) - TRUNC(TO_DATE(bar.h,?))) > ? THEN ? WHEN DECODE(bar.j, NULL, TRUNC(SYSDATE) - TRUNC(TO_DATE(bar.h,?))) > ? THEN ? WHEN DECODE(bar.j, NULL, TRUNC(SYSDATE) - TRUNC(TO_DATE(bar.h,?))) > ? THEN ? WHEN DECODE(bar.j, NULL, TRUNC(SYSDATE) - TRUNC(TO_DATE(bar.h,?))) > ? THEN ? ELSE NULL END AS \"c12\", DECODE(bar.j, NULL, TRUNC(SYSDATE) - TRUNC(TO_DATE(bar.h,?)),NULL) as \"c13\", bar.k AS \"c14\", bar.l ||?||bar.m AS \"c15\", DECODE(bar.n, NULL, NULL,bar.n ||?||bar.o) AS \"c16\", bar.p AS \"c17\", bar.q AS \"c18\", bar.r AS \"c19\", bar.s AS \"c20\", qux.a AS \"c21\", TO_CHAR(TO_DATE(qux.b,?),?) AS \"c22\", DECODE(qux.l,NULL,NULL, qux.l ||?||qux.m) AS \"c23\", bar.a AS \"c24\", TO_CHAR(TO_DATE(bar.j,?),?) AS \"c25\", DECODE(bar.c , ?,?,?, ?, bar.c ) AS \"c26\", bar.y AS y, bar.d, bar.d AS \"c27\" FROM blort.bar , ( SELECT * FROM (SELECT a,a,l,m,b,c, RANK() OVER (PARTITION BY c ORDER BY b DESC) RNK FROM blort.d WHERE y IN (:p)) WHERE RNK = ?) qux, blort.foo WHERE bar.c = qux.c(+) AND bar.x = foo.x AND bar.y IN (:p) and bar.x IN (:x)) )",
			"SELECT count ( * ) FROM ( SELECT c1, c2, c3, c4, c5, c6, c7, c8, c9, c10, c11, c12, c13, c14, c15, c16, c17, c18, c19, c20, c21, c22, c23, c24, c25, c26, c27 FROM ( SELECT bar.y, foo.x, foo.z, DECODE ( foo.a, ? foo.a | | ? | | foo.b ), foo.c, bar.d, bar.e, bar.f, bar.g, TO_DATE ( TO_CHAR ( TO_DATE ( bar.h, ? ) ) ), TO_DATE ( TO_CHAR ( TO_DATE ( bar.i, ? ) ) ), CASE WHEN DECODE ( bar.j, ? TRUNC ( SYSDATE ) - TRUNC ( TO_DATE ( bar.h, ? ) ) ) > ? THEN ? WHEN DECODE ( bar.j, ? TRUNC ( SYSDATE ) - TRUNC ( TO_DATE ( bar.h, ? ) ) ) > ? THEN ? WHEN DECODE ( bar.j, ? TRUNC ( SYSDATE ) - TRUNC ( TO_DATE ( bar.h, ? ) ) ) > ? THEN ? WHEN DECODE ( bar.j, ? TRUNC ( SYSDATE ) - TRUNC ( TO_DATE ( bar.h, ? ) ) ) > ? THEN ? WHEN DECODE ( bar.j, ? TRUNC ( SYSDATE ) - TRUNC ( TO_DATE ( bar.h, ? ) ) ) > ? THEN ? WHEN DECODE ( bar.j, ? TRUNC ( SYSDATE ) - TRUNC ( TO_DATE ( bar.h, ? ) ) ) > ? THEN ? ELSE ? END, DECODE ( bar.j, ? TRUNC ( SYSDATE ) - TRUNC ( TO_DATE ( bar.h, ? ) ) ), bar.k, bar.l | | ? | | bar.m, DECODE ( bar.n, ? bar.n | | ? | | bar.o ), bar.p, bar.q, bar.r, bar.s, qux.a, TO_CHAR ( TO_DATE ( qux.b, ? ) ), DECODE ( qux.l, ? qux.l | | ? | | qux.m ), bar.a, TO_CHAR ( TO_DATE ( bar.j, ? ) ), DECODE ( bar.c, ? bar.c ), bar.y, bar.d, bar.d FROM blort.bar, ( SELECT * FROM ( SELECT a, a, l, m, b, c, RANK ( ) OVER ( PARTITION BY c ORDER BY b DESC ) RNK FROM blort.d WHERE y IN ( :p ) ) WHERE RNK = ? ) qux, blort.foo WHERE bar.c = qux.c ( + ) AND bar.x = foo.x AND bar.y IN ( :p ) and bar.x IN ( :x ) ) ) SELECT count ( * ) FROM ( SELECT c1, c2, c3, c4, c5, c6, c7, c8, c9, c10, c11, c12, c13, c14, c15, c16, c17, c18, c19, c20, c21, c22, c23, c24, c25, c26, c27 FROM ( SELECT bar.y, foo.x, foo.z, DECODE ( foo.a, ? foo.a | | ? | | foo.b ), foo.c, bar.d, bar.e, bar.f, bar.g, TO_DATE ( TO_CHAR ( TO_DATE ( bar.h, ? ) ) ), TO_DATE ( TO_CHAR ( TO_DATE ( bar.i, ? ) ) ), CASE WHEN DECODE ( bar.j, ? TRUNC ( SYSDATE ) - TRUNC ( TO_DATE ( bar.h, ? ) ) ) > ? THEN ? WHEN DECODE ( bar.j, ? TRUNC ( SYSDATE ) - TRUNC ( TO_DATE ( bar.h, ? ) ) ) > ? THEN ? WHEN DECODE ( bar.j, ? TRUNC ( SYSDATE ) - TRUNC ( TO_DATE ( bar.h, ? ) ) ) > ? THEN ? WHEN DECODE ( bar.j, ? TRUNC ( SYSDATE ) - TRUNC ( TO_DATE ( bar.h, ? ) ) ) > ? THEN ? WHEN DECODE ( bar.j, ? TRUNC ( SYSDATE ) - TRUNC ( TO_DATE ( bar.h, ? ) ) ) > ? THEN ? WHEN DECODE ( bar.j, ? TRUNC ( SYSDATE ) - TRUNC ( TO_DATE ( bar.h, ? ) ) ) > ? THEN ? ELSE ? END, DECODE ( bar.j, ? TRUNC ( SYSDATE ) - TRUNC ( TO_DATE ( bar.h, ? ) ) ), bar.k, bar.l | | ? | | bar.m, DECODE ( bar.n, ? bar.n | | ? | | bar.o ), bar.p, bar.q, bar.r, bar.s, qux.a, TO_CHAR ( TO_DATE ( qux.b, ? ) ), DECODE ( qux.l, ? qux.l | | ? | | qux.m ), bar.a, TO_CHAR ( TO_DATE ( bar.j, ? ) ), DECODE ( bar.c, ? bar.c ), bar.y, bar.d, bar.d FROM blort.bar, ( SELECT * FROM ( SELECT a, a, l, m, b, c, RANK ( ) OVER ( PARTITION BY c ORDER BY b DESC ) RNK FROM blort.d WHERE y IN ( :p ) ) WHERE RNK = ? ) qux, blort.foo WHERE bar.c = qux.c ( + ) AND bar.x = foo.x AND bar.y IN ( :p ) and bar.x IN ( :x ) ) )",
		},
	}

	for _, testCase := range queryToExpected {
		s := CassSpan(testCase.in)
		NewObfuscator(nil).Obfuscate(s)
		assert.Equal(testCase.expected, s.Resource)
	}
}

func TestUnicodeDigit(t *testing.T) {
	hangStr := "٩"
	o := NewObfuscator(nil)
	o.ObfuscateSQLString(hangStr)
}

// TestToUpper contains test data lifted from Go's bytes/bytes_test.go, but we test
// that our toUpper returns the same values as bytes.ToUpper.
func TestToUpper(t *testing.T) {
	var upperTests = []struct {
		in string
	}{
		{""},
		{"ONLYUPPER"},
		{"abc"},
		{"AbC123"},
		{"azAZ09_"},
		{"longStrinGwitHmixofsmaLLandcAps"},
		{"long\u0250string\u0250with\u0250nonascii\u2C6Fchars"},
		{"\u0250\u0250\u0250\u0250\u0250"}, // grows one byte per char
		{"a\u0080\U0010FFFF"},              // test utf8.RuneSelf and utf8.MaxRune
	}
	for name, tf := range map[string]func(in []byte) []byte{
		"nil-dst": func(in []byte) []byte {
			return toUpper(in, nil)
		},
		"empty-dst": func(in []byte) []byte {
			return toUpper(in, make([]byte, 0))
		},
		"small-dst": func(in []byte) []byte {
			return toUpper(in, make([]byte, 2))
		},
		"big-dst": func(in []byte) []byte {
			return toUpper(in, make([]byte, 200))
		},
		"big-cap-dst": func(in []byte) []byte {
			return toUpper(in, make([]byte, 0, 200))
		},
	} {
		t.Run(name, func(t *testing.T) {
			for _, tc := range upperTests {
				expect := bytes.ToUpper([]byte(tc.in))
				actual := tf([]byte(tc.in))
				if !bytes.Equal(actual, expect) {
					t.Errorf("toUpper(%q) = %q; want %q", tc.in, actual, expect)
				}
			}
		})
	}

}
