package storage

import (
	"fmt"

	"github.com/Bremcm/minidb/internal/storage/disk"
	"github.com/Bremcm/minidb/internal/storage/wal"
)

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

	keyCol int
	tree   *BTree
	root   disk.PageID
	pager  *disk.Pager
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
	pager  *disk.Pager
	wal    *wal.WAL
}

func findKeyColumn(cols []Column) (int, error) {
	for i, c := range cols {
		if c.Type == TypeInt {
			return i, nil
		}
	}
	return 0, fmt.Errorf("таблице нужна хотя бы одна INT-колонка для ключа")
}

func (t *Table) AppendRow(row Row) error {
	if len(row) <= t.keyCol {
		return fmt.Errorf("в строке нет ключевой колонки")
	}

	keyVal := row[t.keyCol]
	if keyVal.Type != TypeInt {
		return fmt.Errorf("ключевая колонка должна быть INT, получили %s",
			keyVal.Type)
	}

	data := SerializeRow(row)

	if err := t.tree.Insert(keyVal.Int, data); err != nil {
		return err
	}

	t.root = t.tree.Root()
	return nil
}

func (t *Table) ScanRows(fn func(Row) error) error {
	return t.tree.ScanAll(func(key int64, data []byte) error {
		row, err := DeserializeRow(data)
		if err != nil {
			return fmt.Errorf("ключ %d: %w", key, err)
		}
		return fn(row)
	})
}

func (t *Table) SearchRow(key int64) (Row, bool, error) {
	data, found, err := t.tree.Search(key)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}

	row, err := DeserializeRow(data)
	if err != nil {
		return nil, false, err
	}
	return row, true, nil
}

func (t *Table) KeyColumn() int {
	return t.keyCol
}

func OpenCatalog(path string) (*Catalog, error) {
	dm, err := disk.NewDiskManager(path)
	if err != nil {
		return nil, err
	}

	pager := disk.NewPager(dm)

	walPath := path + ".wal"

	w, err := wal.Open(walPath)
	if err != nil {
		return nil, fmt.Errorf("открытие журнала: %w", err)
	}
	pager.SetLogger(w)

	c := &Catalog{
		tables: make(map[string]*Table),
		pager:  pager,
		wal:    w,
	}

	if dm.NumPages() == 0 {
		id, _, err := pager.AllocatePage()
		if err != nil {
			return nil, err
		}
		if id != 0 {
			return nil, fmt.Errorf("страница каталога должна быть 0, получили %d", id)
		}
		return c, nil
	}

	page, err := pager.FetchPage(0)
	if err != nil {
		return nil, fmt.Errorf("чтение каталога: %w", err)
	}

	if disk.NumRows(page) > 0 {
		data, err := disk.ReadRow(page, 0)
		if err != nil {
			return nil, fmt.Errorf("чтение каталога: %w", err)
		}

		tables, err := deserializeCatalog(data)
		if err != nil {
			return nil, err
		}

		for _, t := range tables {
			t.pager = pager
			t.tree = OpenBTree(pager, t.root)
		}
		c.tables = tables
	}

	return c, nil
}

func (c *Catalog) saveCatalog() error {
	data := serializeCatalog(c.tables)

	page, err := c.pager.FetchPage(0)
	if err != nil {
		return err
	}

	disk.InitPage(page)

	if _, err := disk.InsertRow(page, data); err != nil {
		return fmt.Errorf("каталог не влезает в страницу: %w", err)
	}

	c.pager.MarkDirty(0)
	return nil
}

func (c *Catalog) CreateTable(name string, cols []Column) error {
	if _, exists := c.tables[name]; exists {
		return fmt.Errorf("таблица %q уже существует", name)
	}

	keyCol, err := findKeyColumn(cols)
	if err != nil {
		return err
	}

	tree, err := NewBTree(c.pager)
	if err != nil {
		return err
	}

	c.tables[name] = &Table{
		Name:    name,
		Columns: cols,
		keyCol:  keyCol,
		tree:    tree,
		root:    tree.Root(),
		pager:   c.pager,
	}

	return c.saveCatalog()
}

func (c *Catalog) GetTable(name string) (*Table, error) {
	t, ok := c.tables[name]
	if !ok {
		return nil, fmt.Errorf("таблица %q не найдена", name)
	}
	return t, nil
}

func (c *Catalog) Save() error {
	if err := c.saveCatalog(); err != nil {
		return err
	}
	return c.pager.FlushAll()
}

func (c *Catalog) Close() error {
	if err := c.saveCatalog(); err != nil {
		return err
	}
	if err := c.pager.Close(); err != nil {
		return err
	}
	if c.wal != nil {
		return c.wal.Close()
	}
	return nil
}
