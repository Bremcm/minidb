package disk

import (
	"bytes"
	"testing"
)

func TestInitPage(t *testing.T) {
	var p Page
	InitPage(&p)

	if NumRows(&p) != 0 {
		t.Errorf("новая страница должна быть пустой, слотов: %d", NumRows(&p))
	}

	// Свободно всё, кроме заголовка.
	want := uint32(PageSize - headerSize)
	if got := FreeSpace(&p); got != want {
		t.Errorf("свободно %d, ожидали %d", got, want)
	}
}

func TestInsertAndRead(t *testing.T) {
	var p Page
	InitPage(&p)

	rows := [][]byte{
		[]byte("первая строка"),
		[]byte("вторая"),
		[]byte("третья строка подлиннее"),
	}

	// Вставляем.
	for i, data := range rows {
		slot, err := InsertRow(&p, data)
		if err != nil {
			t.Fatalf("вставка %d: %v", i, err)
		}
		if slot != uint32(i) {
			t.Errorf("вставка %d: получили слот %d", i, slot)
		}
	}

	if NumRows(&p) != 3 {
		t.Fatalf("ожидали 3 строки, получили %d", NumRows(&p))
	}

	// Читаем обратно.
	for i, want := range rows {
		got, err := ReadRow(&p, uint32(i))
		if err != nil {
			t.Fatalf("чтение %d: %v", i, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("слот %d: получили %q, ожидали %q", i, got, want)
		}
	}
}

// Разная длина строк не должна ломать границы.
func TestVariableLengths(t *testing.T) {
	var p Page
	InitPage(&p)

	lengths := []int{1, 100, 5, 250, 2}

	for _, n := range lengths {
		data := bytes.Repeat([]byte{byte(n)}, n)
		if _, err := InsertRow(&p, data); err != nil {
			t.Fatalf("вставка длины %d: %v", n, err)
		}
	}

	for i, n := range lengths {
		got, err := ReadRow(&p, uint32(i))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != n {
			t.Fatalf("слот %d: длина %d, ожидали %d", i, len(got), n)
		}
		for j, b := range got {
			if b != byte(n) {
				t.Fatalf("слот %d байт %d: %d, ожидали %d", i, j, b, byte(n))
			}
		}
	}
}

// Заполняем страницу до отказа — вставка должна аккуратно отказать.
func TestPageFull(t *testing.T) {
	var p Page
	InitPage(&p)

	data := make([]byte, 100)
	count := 0

	for {
		_, err := InsertRow(&p, data)
		if err != nil {
			break // место кончилось — это ожидаемо
		}
		count++
		if count > 1000 {
			t.Fatal("страница не заполняется — бесконечная вставка")
		}
	}

	// Примерная оценка: (4096-8) / (100+8) ≈ 37
	if count < 30 || count > 40 {
		t.Errorf("вместилось %d строк по 100 байт, ожидали около 37", count)
	}

	// Всё вставленное должно читаться.
	for i := 0; i < count; i++ {
		if _, err := ReadRow(&p, uint32(i)); err != nil {
			t.Fatalf("слот %d не читается после заполнения: %v", i, err)
		}
	}
}

// Слишком большая строка не влезает в пустую страницу.
func TestTooLarge(t *testing.T) {
	var p Page
	InitPage(&p)

	data := make([]byte, PageSize)
	if _, err := InsertRow(&p, data); err == nil {
		t.Error("ожидали ошибку при вставке строки размером со страницу")
	}
}

func TestReadInvalidSlot(t *testing.T) {
	var p Page
	InitPage(&p)

	InsertRow(&p, []byte("одна строка"))

	if _, err := ReadRow(&p, 1); err == nil {
		t.Error("чтение несуществующего слота должно давать ошибку")
	}
	if _, err := ReadRow(&p, 999); err == nil {
		t.Error("чтение слота 999 должно давать ошибку")
	}
}

// Страница переживает запись на диск и чтение обратно.
func TestPageSurvivesDisk(t *testing.T) {
	dm, err := NewDiskManager(tempDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer dm.Close()

	var p Page
	InitPage(&p)
	InsertRow(&p, []byte("данные на диске"))
	InsertRow(&p, []byte("вторая запись"))

	id, _ := dm.AllocatePage()
	if err := dm.WritePage(id, &p); err != nil {
		t.Fatal(err)
	}

	var readBack Page
	if err := dm.ReadPage(id, &readBack); err != nil {
		t.Fatal(err)
	}

	if NumRows(&readBack) != 2 {
		t.Fatalf("после диска слотов %d, ожидали 2", NumRows(&readBack))
	}

	got, _ := ReadRow(&readBack, 0)
	if !bytes.Equal(got, []byte("данные на диске")) {
		t.Errorf("после диска слот 0: %q", got)
	}
}
