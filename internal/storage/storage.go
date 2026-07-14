package storage

import "fmt"

type ValueType int

const (
	TypeNull ValueType = iota
	TypeInt
	TypeText
)

func (vt ValueType) String() string {
	switch vt {
	case TypeNull:
		return "NULL"
	case TypeInt:
		return "INT"
	case TypeText:
		return "TEXT"
	}
	return "UNKNOWN"
}

type Value struct {
	Type ValueType
	Int  int64
	Text string
}

func NewInt(v int64) Value   { return Value{Type: TypeInt, Int: v} }
func NewText(v string) Value { return Value{Type: TypeText, Text: v} }
func NewNull() Value         { return Value{Type: TypeNull} }

func (v Value) String() string {
	switch v.Type {
	case TypeInt:
		return fmt.Sprintf("%d", v.Int)
	case TypeText:
		return v.Text
	case TypeNull:
		return "NULL"
	}
	return "?"
}

type Row []Value

type Column struct {
	Name string
	Type ValueType
}

type Table struct {
	Name    string
	Columns []Column
	Rows    []Row
}

func (t *Table) ColumnIndex(name string) (int, bool) {
	for i, col := range t.Columns {
		if col.Name == name {
			return i, true
		}
	}
	return 0, false
}

type Catalog struct {
	tables map[string]*Table
}

func NewCatalog() *Catalog {
	return &Catalog{
		tables: make(map[string]*Table),
	}
}

func (c *Catalog) CreateTable(name string, cols []Column) error {
	if _, exists := c.tables[name]; exists {
		return fmt.Errorf("таблица %q уже существует", name)
	}

	c.tables[name] = &Table{
		Name:    name,
		Columns: cols,
		Rows:    []Row{},
	}
	return nil
}

func (c *Catalog) GetTable(name string) (*Table, error) {
	t, ok := c.tables[name]
	if !ok {
		return nil, fmt.Errorf("таблица %q не найдена", name)
	}
	return t, nil
}
