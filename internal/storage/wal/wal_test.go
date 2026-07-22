package wal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Bremcm/minidb/internal/storage/disk"
)

func tempWAL(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "test.wal")
}

func TestEmptyLog(t *testing.T) {
	path := tempWAL(t)

	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	w.Close()

	records, err := ReadAll(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Errorf("в пустом журнале %d записей", len(records))
	}
}

func TestMissingFile(t *testing.T) {
	// Несуществующий журнал — не ошибка.
	records, err := ReadAll(filepath.Join(t.TempDir(), "nosuch.wal"))
	if err != nil {
		t.Fatalf("отсутствие журнала не должно быть ошибкой: %v", err)
	}
	if records != nil {
		t.Errorf("ожидали nil, получили %v", records)
	}
}

func TestWriteAndRead(t *testing.T) {
	path := tempWAL(t)

	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	txID, err := w.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if txID != 1 {
		t.Errorf("первая транзакция должна быть 1, получили %d", txID)
	}

	// Две страницы с узнаваемым содержимым.
	var page1, page2 disk.Page
	for i := range page1 {
		page1[i] = byte(i % 256)
	}
	copy(page2[:], []byte("вторая страница"))

	if err := w.WritePage(txID, 5, &page1); err != nil {
		t.Fatal(err)
	}
	if err := w.WritePage(txID, 7, &page2); err != nil {
		t.Fatal(err)
	}
	if err := w.Commit(txID); err != nil {
		t.Fatal(err)
	}
	w.Close()

	// Читаем обратно.
	records, err := ReadAll(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(records) != 4 {
		t.Fatalf("ожидали 4 записи (BEGIN, 2 PAGE, COMMIT), получили %d",
			len(records))
	}

	if records[0].Type != RecordBegin || records[0].TxID != txID {
		t.Errorf("запись 0: %+v", records[0])
	}
	if records[1].Type != RecordPage || records[1].PageID != 5 {
		t.Errorf("запись 1: тип %d, страница %d", records[1].Type, records[1].PageID)
	}
	if records[1].Page != page1 {
		t.Error("содержимое страницы 5 не совпадает")
	}
	if records[2].PageID != 7 || records[2].Page != page2 {
		t.Error("содержимое страницы 7 не совпадает")
	}
	if records[3].Type != RecordCommit || records[3].TxID != txID {
		t.Errorf("запись 3: %+v", records[3])
	}
}

func TestMultipleTransactions(t *testing.T) {
	path := tempWAL(t)

	w, _ := Open(path)

	var page disk.Page

	for i := 0; i < 3; i++ {
		txID, err := w.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if txID != uint64(i+1) {
			t.Errorf("транзакция %d: получили id %d", i, txID)
		}

		page[0] = byte(i)
		w.WritePage(txID, disk.PageID(i), &page)
		w.Commit(txID)
	}
	w.Close()

	records, _ := ReadAll(path)

	// 3 транзакции × 3 записи = 9
	if len(records) != 9 {
		t.Fatalf("ожидали 9 записей, получили %d", len(records))
	}
}

// Обрезанный журнал: последняя запись недописана.
func TestTruncatedLog(t *testing.T) {
	path := tempWAL(t)

	w, _ := Open(path)
	txID, _ := w.Begin()

	var page disk.Page
	w.WritePage(txID, 1, &page)
	w.Commit(txID)
	w.Close()

	// Обрезаем файл на середине последней записи.
	info, _ := os.Stat(path)
	newSize := info.Size() - 5 // отрезаем 5 байт от COMMIT

	if err := os.Truncate(path, newSize); err != nil {
		t.Fatal(err)
	}

	records, err := ReadAll(path)
	if err != nil {
		t.Fatalf("чтение обрезанного журнала не должно быть ошибкой: %v", err)
	}

	// COMMIT должен отвалиться, BEGIN и PAGE остаться.
	if len(records) != 2 {
		t.Fatalf("ожидали 2 целые записи, получили %d", len(records))
	}
	if records[len(records)-1].Type == RecordCommit {
		t.Error("обрезанный COMMIT не должен читаться")
	}
}

// Повреждённые байты внутри записи ловятся контрольной суммой.
func TestCorruptedRecord(t *testing.T) {
	path := tempWAL(t)

	w, _ := Open(path)
	txID, _ := w.Begin()

	var page disk.Page
	copy(page[:], []byte("оригинальные данные"))
	w.WritePage(txID, 1, &page)
	w.Commit(txID)
	w.Close()

	// Портим байт внутри данных страницы.
	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		t.Fatal(err)
	}
	// BEGIN(13) + тип(1) + txID(8) + pageID(4) = 26, дальше данные
	if _, err := f.WriteAt([]byte{0xFF}, 30); err != nil {
		t.Fatal(err)
	}
	f.Close()

	records, _ := ReadAll(path)

	// BEGIN прочитается, PAGE — нет (сумма не сойдётся).
	if len(records) != 1 {
		t.Fatalf("ожидали 1 запись до повреждения, получили %d", len(records))
	}
	if records[0].Type != RecordBegin {
		t.Errorf("первая запись должна быть BEGIN, получили %d", records[0].Type)
	}
}

func TestTruncate(t *testing.T) {
	path := tempWAL(t)

	w, _ := Open(path)
	txID, _ := w.Begin()

	var page disk.Page
	w.WritePage(txID, 1, &page)
	w.Commit(txID)

	if err := w.Truncate(); err != nil {
		t.Fatal(err)
	}
	w.Close()

	records, _ := ReadAll(path)
	if len(records) != 0 {
		t.Errorf("после Truncate ожидали 0 записей, получили %d", len(records))
	}
}
