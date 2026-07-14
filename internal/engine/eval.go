package engine

import (
	"fmt"

	"github.com/Bremcm/minidb/internal/ast"
	"github.com/Bremcm/minidb/internal/lexer"
	"github.com/Bremcm/minidb/internal/storage"
)

type evalContext struct {
	table *storage.Table
	row   storage.Row
}

func eval(expr ast.Expression, ctx *evalContext) (storage.Value, error) {
	switch e := expr.(type) {

	case *ast.IntegerLiteral:
		return storage.NewInt(e.Value), nil

	case *ast.StringLiteral:
		return storage.NewText(e.Value), nil

	case *ast.Identifier:
		idx, ok := ctx.table.ColumnIndex(e.Name)
		if !ok {
			return storage.Value{}, fmt.Errorf("нет колонки %q в таблице %q",
				e.Name, ctx.table.Name)
		}
		return ctx.row[idx], nil

	case *ast.BinaryExpression:
		return evalBinary(e, ctx)

	default:
		return storage.Value{}, fmt.Errorf("не умею вычислять %T", expr)
	}
}

func evalBinary(e *ast.BinaryExpression, ctx *evalContext) (storage.Value, error) {
	if e.Operator == lexer.AND || e.Operator == lexer.OR {
		return evalLogical(e, ctx)
	}

	left, err := eval(e.Left, ctx)
	if err != nil {
		return storage.Value{}, err
	}

	right, err := eval(e.Right, ctx)
	if err != nil {
		return storage.Value{}, err
	}

	return compare(left, e.Operator, right)
}

func evalLogical(e *ast.BinaryExpression, ctx *evalContext) (storage.Value, error) {
	left, err := eval(e.Left, ctx)
	if err != nil {
		return storage.Value{}, err
	}

	leftTrue := isTruthy(left)

	if e.Operator == lexer.AND && !leftTrue {
		return storage.NewInt(0), nil
	}
	if e.Operator == lexer.OR && leftTrue {
		return storage.NewInt(1), nil
	}

	right, err := eval(e.Right, ctx)
	if err != nil {
		return storage.Value{}, err
	}

	return boolValue(isTruthy(right)), nil
}

func isTruthy(v storage.Value) bool {
	switch v.Type {
	case storage.TypeNull:
		return false
	case storage.TypeInt:
		return v.Int != 0
	case storage.TypeText:
		return v.Text != ""
	}
	return false
}

func boolValue(b bool) storage.Value {
	if b {
		return storage.NewInt(1)
	}
	return storage.NewInt(0)
}

func compare(left storage.Value, op lexer.TokenType, right storage.Value) (storage.Value, error) {
	if left.Type == storage.TypeNull || right.Type == storage.TypeNull {
		return storage.NewNull(), nil
	}

	if left.Type != right.Type {
		return storage.Value{}, fmt.Errorf(
			"нельзя сравнить %s и %s", left.Type, right.Type)
	}

	var cmp int

	switch left.Type {
	case storage.TypeInt:
		switch {
		case left.Int < right.Int:
			cmp = -1
		case left.Int > right.Int:
			cmp = 1
		default:
			cmp = 0
		}

	case storage.TypeText:
		switch {
		case left.Text < right.Text:
			cmp = -1
		case left.Text > right.Text:
			cmp = 1
		default:
			cmp = 0
		}

	default:
		return storage.Value{}, fmt.Errorf("не умею сравнивать %s", left.Type)
	}

	switch op {
	case lexer.EQ:
		return boolValue(cmp == 0), nil
	case lexer.NEQ:
		return boolValue(cmp != 0), nil
	case lexer.LT:
		return boolValue(cmp < 0), nil
	case lexer.GT:
		return boolValue(cmp > 0), nil
	case lexer.LTE:
		return boolValue(cmp <= 0), nil
	case lexer.GTE:
		return boolValue(cmp >= 0), nil
	}

	return storage.Value{}, fmt.Errorf("неизвестный оператор %s", op)
}
