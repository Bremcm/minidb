package engine

import (
	"fmt"

	"github.com/Bremcm/minidb/internal/ast"
	"github.com/Bremcm/minidb/internal/lexer"
	"github.com/Bremcm/minidb/internal/storage"
)

type Engine struct {
	catalog *storage.Catalog
}

func New(path string) (*Engine, error) {
	cat, err := storage.OpenCatalog(path)
	if err != nil {
		return nil, err
	}
	return &Engine{catalog: cat}, nil
}

func (e *Engine) Save() error {
	return e.catalog.Save()
}

func (e *Engine) Close() error {
	return e.catalog.Close()
}

type Result struct {
	Columns []string
	Rows    []storage.Row
	Message string
}

func (e *Engine) Execute(stmt ast.Statement) (*Result, error) {
	switch s := stmt.(type) {
	case *ast.CreateTableStatement:
		return e.execCreateTable(s)
	case *ast.InsertStatement:
		return e.execInsert(s)
	case *ast.SelectStatement:
		return e.execSelect(s)
	default:
		return nil, fmt.Errorf("не умею выполнять %T", stmt)
	}
}

func (e *Engine) execCreateTable(s *ast.CreateTableStatement) (*Result, error) {
	cols := make([]storage.Column, 0, len(s.Columns))

	for _, c := range s.Columns {
		var vt storage.ValueType

		switch c.Type {
		case "INT":
			vt = storage.TypeInt
		case "TEXT":
			vt = storage.TypeText
		default:
			return nil, fmt.Errorf("неизвестный тип %q", c.Type)
		}

		cols = append(cols, storage.Column{Name: c.Name, Type: vt})
	}

	if err := e.catalog.CreateTable(s.Table, cols); err != nil {
		return nil, err
	}

	return &Result{
		Message: fmt.Sprintf("таблица %q создана", s.Table),
	}, nil
}

func (e *Engine) execInsert(s *ast.InsertStatement) (*Result, error) {
	table, err := e.catalog.GetTable(s.Table)
	if err != nil {
		return nil, err
	}

	if len(s.Values) != len(table.Columns) {
		return nil, fmt.Errorf(
			"таблица %q имеет %d колонок, а передано %d значений",
			s.Table, len(table.Columns), len(s.Values))
	}

	row := make(storage.Row, len(s.Values))

	ctx := &evalContext{table: table}

	for i, expr := range s.Values {
		val, err := eval(expr, ctx)
		if err != nil {
			return nil, err
		}

		expected := table.Columns[i].Type
		if val.Type != expected && val.Type != storage.TypeNull {
			return nil, fmt.Errorf(
				"колонка %q имеет тип %s, а значение — %s",
				table.Columns[i].Name, expected, val.Type)
		}

		row[i] = val
	}

	if err := table.AppendRow(row); err != nil {
		return nil, err
	}

	return &Result{Message: "вставлена 1 строка"}, nil
}

func (e *Engine) execSelect(s *ast.SelectStatement) (*Result, error) {
	table, err := e.catalog.GetTable(s.From)
	if err != nil {
		return nil, err
	}

	var outIdx []int
	var outNames []string

	if key, ok := indexableKey(s.Where, table); ok {
		row, found, err := table.SearchRow(key)
		if err != nil {
			return nil, err
		}

		result := &Result{Columns: outNames, Rows: []storage.Row{}}
		if found {
			outRow := make(storage.Row, len(outIdx))
			for i, idx := range outIdx {
				outRow[i] = row[idx]
			}
			result.Rows = append(result.Rows, outRow)
		}
		return result, nil
	}

	if s.Columns == nil {
		outIdx = make([]int, len(table.Columns))
		outNames = make([]string, len(table.Columns))

		for i, c := range table.Columns {
			outIdx[i] = i
			outNames[i] = c.Name
		}
	} else {
		outIdx = make([]int, 0, len(s.Columns))
		outNames = make([]string, 0, len(s.Columns))

		for _, expr := range s.Columns {
			ident, ok := expr.(*ast.Identifier)
			if !ok {
				return nil, fmt.Errorf("в SELECT пока поддерживаются только имена колонок")
			}

			idx, found := table.ColumnIndex(ident.Name)
			if !found {
				return nil, fmt.Errorf("нет колонки %q в таблице %q",
					ident.Name, s.From)
			}

			outIdx = append(outIdx, idx)
			outNames = append(outNames, ident.Name)
		}
	}

	result := &Result{
		Columns: outNames,
		Rows:    []storage.Row{},
	}

	ctx := &evalContext{table: table}

	err = table.ScanRows(func(row storage.Row) error {
		ctx.row = row

		if s.Where != nil {
			val, err := eval(s.Where, ctx)
			if err != nil {
				return err
			}
			if !isTruthy(val) {
				return nil
			}
		}

		outRow := make(storage.Row, len(outIdx))
		for i, idx := range outIdx {
			outRow[i] = row[idx]
		}

		result.Rows = append(result.Rows, outRow)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func indexableKey(where ast.Expression, table *storage.Table) (int64, bool) {
	if where == nil {
		return 0, false
	}

	bin, ok := where.(*ast.BinaryExpression)
	if !ok || bin.Operator != lexer.EQ {
		return 0, false
	}

	keyColName := table.Columns[table.KeyColumn()].Name

	if id, ok := bin.Left.(*ast.Identifier); ok && id.Name == keyColName {
		if lit, ok := bin.Right.(*ast.IntegerLiteral); ok {
			return lit.Value, true
		}
	}

	if id, ok := bin.Right.(*ast.Identifier); ok && id.Name == keyColName {
		if lit, ok := bin.Left.(*ast.IntegerLiteral); ok {
			return lit.Value, true
		}
	}

	return 0, false
}
