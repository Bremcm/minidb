package disk

import (
	"path/filepath"
	"testing"
)

// tempDB создаёт временный файл БД, который удалится после теста.
func tempDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir() // Go сам удалит эту папку после теста
	return filepath.Join(dir, "test.db")
}

func TestAllocateAndRead(t *testing.T) {
	dm, err := NewDiskManager(tempDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer dm.Close()

	// Выделяем страницу.
	id, err := dm.AllocatePage()
	if err != nil {
		t.Fatal(err)
	}
	if id != 0 {
		t.Fatalf("первая страница должна иметь id 0, получили %d", id)
	}

	// Пишем в неё узнаваемый узор.
	var page Page
	for i := range page {
		page[i] = byte(i % 256)
	}
	if err := dm.WritePage(id, &page); err != nil {
		t.Fatal(err)
	}

	// Читаем обратно в другой буфер.
	var readBack Page
	if err := dm.ReadPage(id, &readBack); err != nil {
		t.Fatal(err)
	}

	// Должно совпасть байт в байт.
	if page != readBack {
		t.Fatal("прочитанная страница не совпадает с записанной")
	}
}

func TestPersistence(t *testing.T) {
	path := tempDB(t)

	// Первая сессия: создаём, пишем, закрываем.
	dm, err := NewDiskManager(path)
	if err != nil {
		t.Fatal(err)
	}

	id, _ := dm.AllocatePage()

	var page Page
	copy(page[:], []byte("привет с диска"))
	dm.WritePage(id, &page)
	dm.Sync()
	dm.Close()

	// Вторая сессия: открываем заново, читаем.
	dm2, err := NewDiskManager(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dm2.Close()

	if dm2.NumPages() != 1 {
		t.Fatalf("ожидали 1 страницу после переоткрытия, получили %d",
			dm2.NumPages())
	}

	var readBack Page
	if err := dm2.ReadPage(id, &readBack); err != nil {
		t.Fatal(err)
	}

	expected := []byte("привет с диска")
	if string(readBack[:len(expected)]) != string(expected) {
		t.Fatalf("данные не пережили переоткрытие: %q",
			string(readBack[:len(expected)]))
	}
}

func TestMultiplePages(t *testing.T) {
	dm, err := NewDiskManager(tempDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer dm.Close()

	// Выделяем три страницы, в каждую пишем её номер.
	for want := PageID(0); want < 3; want++ {
		id, err := dm.AllocatePage()
		if err != nil {
			t.Fatal(err)
		}
		if id != want {
			t.Fatalf("ожидали id %d, получили %d", want, id)
		}

		var page Page
		page[0] = byte(id)
		dm.WritePage(id, &page)
	}

	// Проверяем, что страницы не перепутались.
	for id := PageID(0); id < 3; id++ {
		var page Page
		dm.ReadPage(id, &page)
		if page[0] != byte(id) {
			t.Fatalf("страница %d содержит %d вместо %d", id, page[0], id)
		}
	}
}
