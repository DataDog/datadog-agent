// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pgsyntax

import (
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	pg_query "github.com/pganalyze/pg_query_go/v6"
)

const maxTraversalStackDepth = 100000

// Return true to continue traversal for this subtree, false to stop
type Visit func(node *pg_query.Node) bool

// Generic traversal of an AST of pg_query.Nodes
//
// This pattern was chosen in favor of a typical visitor pattern because
// non-local types can't be extended to implement interfaces. Using a
// visit function allows for a very simple interface, and offloads the
// knowledge of individual node types to the visit function.
func Traverse(node *pg_query.Node, visit Visit) {
	if node == nil {
		return
	}

	depth := -1
	stack := []*pg_query.Node{node}
	for len(stack) > 0 {
		if depth > maxTraversalStackDepth {
			log.Warnf("Maximum AST traversal depth exceeded\n")
			return
		}

		depth += 1
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if curr == nil {
			continue
		}

		switch curr.GetNode().(type) {
		/*
			These are currently unhandled, either because they don't have any
			child nodes or because they are nodes that appear in the Postgres source
			but not in the pg_query parser wrapper.

			case *pg_query.Node_Var:
			case *pg_query.Node_AConst:
			case *pg_query.Node_Param:
			case *pg_query.Node_CaseTestExpr:
			case *pg_query.Node_SqlvalueFunction:
			case T_IndexClause:
			case T_JsonValueExpr:
			case T_JsonConstructorExpr:
			case T_JsonIsPredicate:
			case T_JsonKeyValue:
			case T_JsonObjectConstructor:
			case T_JsonArrayConstructor:
			case T_JsonArrayQueryConstructor:
			case T_JsonAggConstructor:
			case T_JsonObjectAgg:
			case T_JsonArrayAgg:
			case T_PlaceHolderVar:
			case T_AppendRelInfo:
		*/
		case *pg_query.Node_ExplainStmt:
			if !visit(curr) {
				continue
			}

			explainStmt := curr.GetExplainStmt()
			if explainStmt.Query != nil {
				stack = append(stack, explainStmt.Query)
			}
		case *pg_query.Node_InsertStmt:
			if !visit(curr) {
				continue
			}

			insertStmt := curr.GetInsertStmt()
			stack = append(stack, &pg_query.Node{Node: &pg_query.Node_RangeVar{RangeVar: insertStmt.Relation}})
			stack = append(stack, insertStmt.Cols...)
			stack = append(stack, insertStmt.SelectStmt)
			stack = append(stack, &pg_query.Node{Node: &pg_query.Node_OnConflictClause{OnConflictClause: insertStmt.OnConflictClause}})
			stack = append(stack, insertStmt.ReturningList...)
			if insertStmt.WithClause != nil {
				stack = append(stack, &pg_query.Node{Node: &pg_query.Node_WithClause{WithClause: insertStmt.WithClause}})
			}
		case *pg_query.Node_DeleteStmt:
			if !visit(curr) {
				continue
			}

			deleteStmt := curr.GetDeleteStmt()
			stack = append(stack, &pg_query.Node{Node: &pg_query.Node_RangeVar{RangeVar: deleteStmt.Relation}})
			stack = append(stack, deleteStmt.UsingClause...)
			stack = append(stack, deleteStmt.WhereClause)
			stack = append(stack, deleteStmt.ReturningList...)
			if deleteStmt.WithClause != nil {
				stack = append(stack, &pg_query.Node{Node: &pg_query.Node_WithClause{WithClause: deleteStmt.WithClause}})
			}
		case *pg_query.Node_UpdateStmt:
			if !visit(curr) {
				continue
			}

			updateStmt := curr.GetUpdateStmt()

			if updateStmt.Relation != nil {
				stack = append(stack, &pg_query.Node{Node: &pg_query.Node_RangeVar{RangeVar: updateStmt.Relation}})
			}
			stack = append(stack, updateStmt.TargetList...)
			stack = append(stack, updateStmt.WhereClause)
			stack = append(stack, updateStmt.FromClause...)
			stack = append(stack, updateStmt.ReturningList...)
			if updateStmt.WithClause != nil {
				stack = append(stack, &pg_query.Node{Node: &pg_query.Node_WithClause{WithClause: updateStmt.WithClause}})
			}

		case *pg_query.Node_SelectStmt:
			if !visit(curr) {
				continue
			}

			selectStmt := curr.GetSelectStmt()
			stack = append(stack, selectStmt.DistinctClause...)
			stack = append(stack, selectStmt.TargetList...)
			stack = append(stack, selectStmt.FromClause...)
			stack = append(stack, selectStmt.WhereClause)
			stack = append(stack, selectStmt.GroupClause...)
			stack = append(stack, selectStmt.HavingClause)
			stack = append(stack, selectStmt.WindowClause...)
			stack = append(stack, selectStmt.ValuesLists...)
			stack = append(stack, selectStmt.SortClause...)
			stack = append(stack, selectStmt.LimitCount)
			stack = append(stack, selectStmt.LimitOffset)
			stack = append(stack, selectStmt.LockingClause...)
			if selectStmt.IntoClause != nil {
				stack = append(stack, &pg_query.Node{Node: &pg_query.Node_IntoClause{IntoClause: selectStmt.IntoClause}})
			}

			if selectStmt.WithClause != nil {
				stack = append(stack, &pg_query.Node{Node: &pg_query.Node_WithClause{WithClause: selectStmt.WithClause}})
			}

			if selectStmt.Larg != nil && selectStmt.Rarg != nil {
				stack = append(stack, &pg_query.Node{Node: &pg_query.Node_SelectStmt{SelectStmt: selectStmt.Rarg}})
			}

			if selectStmt.Larg != nil {
				stack = append(stack, &pg_query.Node{Node: &pg_query.Node_SelectStmt{SelectStmt: selectStmt.Larg}})
			}
		case *pg_query.Node_WithClause:
			withClause := curr.GetWithClause()
			if withClause == nil {
				log.Warnf("No with clause found in WithClause node: %+v", curr)
				continue
			}
			if !visit(curr) {
				continue
			}

			stack = append(stack, withClause.Ctes...)
		case *pg_query.Node_ResTarget:
			if !visit(curr) {
				continue
			}

			resTarget := curr.GetResTarget()

			stack = append(stack, resTarget.Val)
			stack = append(stack, resTarget.Indirection...)
		case *pg_query.Node_WithCheckOption:
			withCheckOption := curr.GetWithCheckOption()
			if !visit(curr) {
				continue
			}

			stack = append(stack, withCheckOption.Qual)
		case *pg_query.Node_Aggref:
			aggref := curr.GetAggref()
			if !visit(curr) {
				continue
			}

			stack = append(stack, aggref.Aggdirectargs...)
			stack = append(stack, aggref.Args...)
			stack = append(stack, aggref.Aggorder...)
			stack = append(stack, aggref.Aggdistinct...)
			stack = append(stack, aggref.Aggfilter)
		case *pg_query.Node_GroupingFunc:
			groupingFunc := curr.GetGroupingFunc()
			if !visit(curr) {
				continue
			}

			stack = append(stack, groupingFunc.Args...)
		case *pg_query.Node_FuncCall:
			funcCall := curr.GetFuncCall()
			if !visit(curr) {
				continue
			}

			stack = append(stack, funcCall.Args...)
			stack = append(stack, funcCall.AggOrder...)
			stack = append(stack, funcCall.AggFilter)
		case *pg_query.Node_WindowFunc:
			windowFunc := curr.GetWindowFunc()
			if !visit(curr) {
				continue
			}

			stack = append(stack, windowFunc.Args...)

			stack = append(stack, windowFunc.Aggfilter)
		case *pg_query.Node_SubscriptingRef:
			subscriptingRef := curr.GetSubscriptingRef()
			if !visit(curr) {
				continue
			}

			stack = append(stack, subscriptingRef.Refupperindexpr...)
			stack = append(stack, subscriptingRef.Reflowerindexpr...)
			stack = append(stack, subscriptingRef.Refassgnexpr)
			stack = append(stack, subscriptingRef.Refexpr)
		case *pg_query.Node_FuncExpr:
			funcExpr := curr.GetFuncExpr()
			if !visit(curr) {
				continue
			}
			stack = append(stack, funcExpr.Args...)
		case *pg_query.Node_NamedArgExpr:
			namedArgExpr := curr.GetNamedArgExpr()
			if !visit(curr) {
				continue
			}
			stack = append(stack, namedArgExpr.Arg)
		case *pg_query.Node_AIndices:
			aIndices := curr.GetAIndices()
			if !visit(curr) {
				continue
			}

			stack = append(stack, aIndices.Lidx)
			stack = append(stack, aIndices.Uidx)
		case *pg_query.Node_OpExpr:
			opExpr := curr.GetOpExpr()
			if !visit(curr) {
				continue
			}

			stack = append(stack, opExpr.Args...)
		case *pg_query.Node_DistinctExpr:
			distinctExpr := curr.GetDistinctExpr()
			if !visit(curr) {
				continue
			}

			stack = append(stack, distinctExpr.Args...)
		case *pg_query.Node_NullIfExpr:
			nullIfExpr := curr.GetNullIfExpr()
			if !visit(curr) {
				continue
			}

			stack = append(stack, nullIfExpr.Args...)
		case *pg_query.Node_ScalarArrayOpExpr:
			scalarArrayOpExpr := curr.GetScalarArrayOpExpr()
			if !visit(curr) {
				continue
			}

			stack = append(stack, scalarArrayOpExpr.Args...)
		case *pg_query.Node_BoolExpr:
			boolExpr := curr.GetBoolExpr()
			if !visit(curr) {
				continue
			}

			stack = append(stack, boolExpr.Args...)
		case *pg_query.Node_ParamRef:
			if !visit(curr) {
				continue
			}
		case *pg_query.Node_SubLink:
			subLink := curr.GetSubLink()
			if !visit(curr) {
				continue
			}
			stack = append(stack, subLink.Testexpr)
			stack = append(stack, subLink.Subselect)
		case *pg_query.Node_SubPlan:
			subPlan := curr.GetSubPlan()
			if !visit(curr) {
				continue
			}
			stack = append(stack, subPlan.Testexpr)
			stack = append(stack, subPlan.Args...)
		case *pg_query.Node_AlternativeSubPlan:
			alternativeSubPlan := curr.GetAlternativeSubPlan()
			if !visit(curr) {
				continue
			}
			stack = append(stack, alternativeSubPlan.Subplans...)
		case *pg_query.Node_FieldSelect:
			fieldSelect := curr.GetFieldSelect()
			if !visit(curr) {
				continue
			}
			stack = append(stack, fieldSelect.Arg)
		case *pg_query.Node_FieldStore:
			fieldStore := curr.GetFieldStore()
			if !visit(curr) {
				continue
			}
			stack = append(stack, fieldStore.Arg)
			stack = append(stack, fieldStore.Newvals...)
		case *pg_query.Node_RelabelType:
			relabelType := curr.GetRelabelType()
			if !visit(curr) {
				continue
			}
			stack = append(stack, relabelType.Arg)
		case *pg_query.Node_CoerceViaIo:
			coerceViaIo := curr.GetCoerceViaIo()
			if !visit(curr) {
				continue
			}
			stack = append(stack, coerceViaIo.Arg)
		case *pg_query.Node_ArrayCoerceExpr:
			arrayCoerceExpr := curr.GetArrayCoerceExpr()
			if !visit(curr) {
				continue
			}

			stack = append(stack, arrayCoerceExpr.Arg)
			stack = append(stack, arrayCoerceExpr.Elemexpr)
		case *pg_query.Node_ConvertRowtypeExpr:
			convertRowtypeExpr := curr.GetConvertRowtypeExpr()
			if !visit(curr) {
				continue
			}

			stack = append(stack, convertRowtypeExpr.Arg)
		case *pg_query.Node_CollateExpr:
			collateExpr := curr.GetCollateExpr()
			if !visit(curr) {
				continue
			}
			stack = append(stack, collateExpr.Arg)
		case *pg_query.Node_CaseExpr:
			caseExpr := curr.GetCaseExpr()
			if !visit(curr) {
				continue
			}

			stack = append(stack, caseExpr.Arg)

			stack = append(stack, caseExpr.Args...)

			// Do we need to explicitly check for CaseWhens here?

			stack = append(stack, caseExpr.Defresult)
		case *pg_query.Node_ArrayExpr:
			arrayExpr := curr.GetArrayExpr()
			if !visit(curr) {
				continue
			}

			stack = append(stack, arrayExpr.Elements...)
		case *pg_query.Node_RowExpr:
			rowExpr := curr.GetRowExpr()
			if !visit(curr) {
				continue
			}

			stack = append(stack, rowExpr.Args...)
		case *pg_query.Node_RowCompareExpr:
			rowCompareExpr := curr.GetRowCompareExpr()
			if !visit(curr) {
				continue
			}

			stack = append(stack, rowCompareExpr.Largs...)
			stack = append(stack, rowCompareExpr.Rargs...)
		case *pg_query.Node_CoalesceExpr:
			coalesceExpr := curr.GetCoalesceExpr()
			if !visit(curr) {
				continue
			}

			stack = append(stack, coalesceExpr.Args...)

		case *pg_query.Node_MinMaxExpr:
			minMaxExpr := curr.GetMinMaxExpr()
			if !visit(curr) {
				continue
			}

			stack = append(stack, minMaxExpr.Args...)
		case *pg_query.Node_XmlExpr:
			xmlExpr := curr.GetXmlExpr()
			if !visit(curr) {
				continue
			}

			stack = append(stack, xmlExpr.Args...)
			stack = append(stack, xmlExpr.NamedArgs...)
		case *pg_query.Node_NullTest:
			nullTest := curr.GetNullTest()
			if !visit(curr) {
				continue
			}

			stack = append(stack, nullTest.Arg)
		case *pg_query.Node_BooleanTest:
			booleanTest := curr.GetBooleanTest()
			if !visit(curr) {
				continue
			}

			stack = append(stack, booleanTest.Arg)
		case *pg_query.Node_CoerceToDomain:
			coerceToDomain := curr.GetCoerceToDomain()
			if !visit(curr) {
				continue
			}

			stack = append(stack, coerceToDomain.Arg)
		case *pg_query.Node_TargetEntry:
			targetEntry := curr.GetTargetEntry()
			if !visit(curr) {
				continue
			}

			stack = append(stack, targetEntry.Expr)
		case *pg_query.Node_WindowClause:
			windowClause := curr.GetWindowClause()
			if !visit(curr) {
				continue
			}

			stack = append(stack, windowClause.PartitionClause...)
			stack = append(stack, windowClause.OrderClause...)

			stack = append(stack, windowClause.StartOffset)
			stack = append(stack, windowClause.EndOffset)
		case *pg_query.Node_CtecycleClause:
			ctecycleClause := curr.GetCtecycleClause()
			if !visit(curr) {
				continue
			}

			stack = append(stack, ctecycleClause.CycleMarkValue)
			stack = append(stack, ctecycleClause.CycleMarkDefault)
		case *pg_query.Node_CommonTableExpr:
			commonTableExpr := curr.GetCommonTableExpr()
			if !visit(curr) {
				continue
			}

			stack = append(stack, commonTableExpr.Ctequery)
			if commonTableExpr.SearchClause != nil {
				stack = append(stack, commonTableExpr.SearchClause.SearchColList...)
			}
			if commonTableExpr.CycleClause != nil {
				stack = append(stack, commonTableExpr.CycleClause.CycleColList...)
			}
		case *pg_query.Node_PartitionBoundSpec:
			partitionBoundSpec := curr.GetPartitionBoundSpec()
			if !visit(curr) {
				continue
			}

			stack = append(stack, partitionBoundSpec.Listdatums...)
			stack = append(stack, partitionBoundSpec.Lowerdatums...)
			stack = append(stack, partitionBoundSpec.Upperdatums...)
		case *pg_query.Node_PartitionRangeDatum:
			partitionRangeDatum := curr.GetPartitionRangeDatum()
			if !visit(curr) {
				continue
			}

			stack = append(stack, partitionRangeDatum.Value)
		case *pg_query.Node_List:
			list := curr.GetList()
			if !visit(curr) {
				continue
			}

			stack = append(stack, list.Items...)
		case *pg_query.Node_FromExpr:
			fromExpr := curr.GetFromExpr()
			if !visit(curr) {
				continue
			}

			stack = append(stack, fromExpr.Fromlist...)
			stack = append(stack, fromExpr.Quals)
		case *pg_query.Node_OnConflictExpr:
			onConflictExpr := curr.GetOnConflictExpr()
			if !visit(curr) {
				continue
			}

			stack = append(stack, onConflictExpr.ArbiterElems...)
			stack = append(stack, onConflictExpr.ArbiterWhere)
			stack = append(stack, onConflictExpr.OnConflictSet...)
			stack = append(stack, onConflictExpr.OnConflictWhere)
			stack = append(stack, onConflictExpr.ExclRelTlist...)
		case *pg_query.Node_MergeAction:
			mergeAction := curr.GetMergeAction()
			if !visit(curr) {
				continue
			}

			stack = append(stack, mergeAction.TargetList...)
			stack = append(stack, mergeAction.Qual)
		// case *pg_query.Node_PartitionPruneStepOp:
		case *pg_query.Node_JoinExpr:
			joinExpr := curr.GetJoinExpr()
			if !visit(curr) {
				continue

			}

			stack = append(stack, joinExpr.Larg)
			stack = append(stack, joinExpr.Rarg)
			stack = append(stack, joinExpr.Quals)
		case *pg_query.Node_SetOperationStmt:
			setOperationStmt := curr.GetSetOperationStmt()
			if !visit(curr) {
				continue
			}

			stack = append(stack, setOperationStmt.Larg)
			stack = append(stack, setOperationStmt.Rarg)
		case *pg_query.Node_InferenceElem:
			inferenceElem := curr.GetInferenceElem()
			if !visit(curr) {
				continue
			}

			stack = append(stack, inferenceElem.Expr)
		case *pg_query.Node_RangeTblFunction:
			rangeTblFunction := curr.GetRangeTblFunction()
			if !visit(curr) {
				continue
			}

			stack = append(stack, rangeTblFunction.Funcexpr)
		case *pg_query.Node_TableSampleClause:
			tableSampleClause := curr.GetTableSampleClause()
			if !visit(curr) {
				continue
			}

			stack = append(stack, tableSampleClause.Args...)
			stack = append(stack, tableSampleClause.Repeatable)
		case *pg_query.Node_TableFunc:
			tableFunc := curr.GetTableFunc()
			if !visit(curr) {
				continue
			}

			stack = append(stack, tableFunc.Docexpr)
			stack = append(stack, tableFunc.Rowexpr)
			stack = append(stack, tableFunc.NsUris...)
			stack = append(stack, tableFunc.Colexprs...)
			stack = append(stack, tableFunc.Coldefexprs...)
		case *pg_query.Node_Alias:
			alias := curr.GetAlias()
			if !visit(curr) {
				continue
			}

			stack = append(stack, alias.Colnames...)
		case *pg_query.Node_RangeVar:
			if !visit(curr) {
				continue
			}
		case *pg_query.Node_RangeSubselect:
			if !visit(curr) {
				continue
			}

			rangeSubselect := curr.GetRangeSubselect()
			if rangeSubselect.Subquery != nil {
				stack = append(stack, rangeSubselect.Subquery)
			}
			if rangeSubselect.Alias != nil {
				stack = append(stack, &pg_query.Node{Node: &pg_query.Node_Alias{Alias: rangeSubselect.Alias}})
			}
		case *pg_query.Node_RangeFunction:
			if !visit(curr) {
				continue
			}

			rangeFunction := curr.GetRangeFunction()
			// Functions field contains List nodes that wrap FuncCall nodes
			stack = append(stack, rangeFunction.Functions...)
			stack = append(stack, rangeFunction.Coldeflist...)
			if rangeFunction.Alias != nil {
				stack = append(stack, &pg_query.Node{Node: &pg_query.Node_Alias{Alias: rangeFunction.Alias}})
			}
		case *pg_query.Node_ColumnRef:
			columnRef := curr.GetColumnRef()

			if !visit(curr) {
				continue
			}

			stack = append(stack, columnRef.Fields...)
		case *pg_query.Node_TypeCast:
			typeCast := curr.GetTypeCast()
			if !visit(curr) {
				continue
			}

			stack = append(stack, typeCast.Arg)
		case *pg_query.Node_AExpr:
			aExpr := curr.GetAExpr()
			if !visit(curr) {
				continue
			}

			stack = append(stack, aExpr.Lexpr)
			stack = append(stack, aExpr.Rexpr)
		}
	}
}
