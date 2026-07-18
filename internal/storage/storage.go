package storage

import (
	"fmt"

	"github.com/Bremcm/minidb/internal/storage/disk"
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

	pages []disk.PageID
	pager *disk.Pager
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
}

func OpenCatalog(path string) (*Catalog, error) {
	dm, err := disk.NewDiskManager(path)
	if err != nil {
		return nil, err
	}

	pager := disk.NewPager(dm)

	c := &Catalog{
		tables: make(map[string]*Table),
		pager:  pager,
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

	c.tables[name] = &Table{
		Name:    name,
		Columns: cols,
		pages:   []disk.PageID{},
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
	return c.pager.Close()
}

func (t *Table) AppendRow(row Row) error {
	data := SerializeRow(row)

	if uint32(len(data)) > disk.PageSize/2 {
		return fmt.Errorf("строка слишком велика: %d байт", len(data))
	}

	if len(t.pages) > 0 {
		last := t.pages[len(t.pages)-1]

		page, err := t.pager.FetchPage(last)
		if err != nil {
			return err
		}

		if _, err := disk.InsertRow(page, data); err == nil {
			t.pager.MarkDirty(last)
			return nil
		}
	}

	id, page, err := t.pager.AllocatePage()
	if err != nil {
		return err
	}

	if _, err := disk.InsertRow(page, data); err != nil {
		return fmt.Errorf("строка не влезает даже в пустую страницу: %w", err)
	}

	t.pager.MarkDirty(id)
	t.pages = append(t.pages, id)

	return nil
}

func (t *Table) ScanRows(fn func(Row) error) error {
	for _, pid := range t.pages {
		page, err := t.pager.FetchPage(pid)
		if err != nil {
			return err
		}

		n := disk.NumRows(page)
		for i := uint32(0); i < n; i++ {
			data, err := disk.ReadRow(page, i)
			if err != nil {
				return fmt.Errorf("страница %d слот %d: %w", pid, i, err)
			}

			row, err := DeserializeRow(data)
			if err != nil {
				return fmt.Errorf("страница %d слот %d: %w", pid, i, err)
			}

			if err := fn(row); err != nil {
				return err
			}
		}
	}

	return nil
}
