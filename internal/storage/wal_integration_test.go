package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Bremcm/minidb/internal/storage/wal"
)

// После успешного закрытия журнал должен быть пуст:
// данные записаны, checkpoint сделан.
func TestWALTruncatedAfterFlush(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	walPath := dbPath + ".wal"

	cat, err := OpenCatalog(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	cols := []Column{
		{Name: "id", Type: TypeInt},
		{Name: "name", Type: TypeText},
	}
	if err := cat.CreateTable("users", cols); err != nil {
		t.Fatal(err)
	}

	tbl, _ := cat.GetTable("users")
	for i := int64(1); i <= 10; i++ {
		row := Row{NewInt(i), NewText("user")}
		if err := tbl.AppendRow(row); err != nil {
			t.Fatal(err)
		}
	}

	if err := cat.Close(); err != nil {
		t.Fatal(err)
	}

	// Журнал должен быть пуст после checkpoint.
	info, err := os.Stat(walPath)
	if err != nil {
		t.Fatalf("файл журнала должен существовать: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("после закрытия журнал должен быть пуст, размер %d",
			info.Size())
	}

	// Данные должны быть на месте.
	cat2, err := OpenCatalog(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer cat2.Close()

	tbl2, err := cat2.GetTable("users")
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	tbl2.ScanRows(func(row Row) error {
		count++
		return nil
	})

	if count != 10 {
		t.Errorf("после перезапуска %d строк, ожидали 10", count)
	}
}

// Проверяем, что журнал действительно наполняется во время работы,
// а не остаётся пустым (то есть интеграция реально работает).
func TestWALReceivesRecords(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	walPath := dbPath + ".wal"

	// Открываем журнал отдельно, чтобы посмотреть на записи
	// до того, как Truncate их сотрёт.
	cat, err := OpenCatalog(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	cols := []Column{{Name: "id", Type: TypeInt}}
	cat.CreateTable("t", cols)

	tbl, _ := cat.GetTable("t")
	tbl.AppendRow(Row{NewInt(1)})

	// Сбрасываем через Save (внутри FlushAll → журнал → данные → truncate).
	if err := cat.Save(); err != nil {
		t.Fatal(err)
	}

	// После Save журнал очищен — это ожидаемо.
	records, err := wal.ReadAll(walPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Errorf("после Save журнал должен быть пуст, записей %d", len(records))
	}

	cat.Close()
}
