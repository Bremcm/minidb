package disk

import (
	"bytes"
	"fmt"
	"testing"
)

func TestSearchKeyEmpty(t *testing.T) {
	var p Page
	InitLeaf(&p)

	pos, found := SearchKey(&p, 42)
	if found {
		t.Error("в пустом узле ничего не должно находиться")
	}
	if pos != 0 {
		t.Errorf("позиция вставки в пустой узел должна быть 0, получили %d", pos)
	}
}

func TestSearchKeyExact(t *testing.T) {
	var p Page
	InitLeaf(&p)

	keys := []int64{10, 30, 50, 70, 90}
	for _, k := range keys {
		if err := LeafInsert(&p, k, []byte(fmt.Sprintf("v%d", k))); err != nil {
			t.Fatal(err)
		}
	}

	for i, k := range keys {
		pos, found := SearchKey(&p, k)
		if !found {
			t.Errorf("ключ %d не найден", k)
		}
		if pos != uint32(i) {
			t.Errorf("ключ %d: позиция %d, ожидали %d", k, pos, i)
		}
	}
}

// Не найденный ключ должен давать позицию для вставки.
func TestSearchKeyInsertPosition(t *testing.T) {
	var p Page
	InitLeaf(&p)

	for _, k := range []int64{10, 30, 50} {
		LeafInsert(&p, k, []byte("x"))
	}

	cases := []struct {
		key     int64
		wantPos uint32
	}{
		{5, 0},   // до всех
		{20, 1},  // между 10 и 30
		{40, 2},  // между 30 и 50
		{100, 3}, // после всех
	}

	for _, c := range cases {
		pos, found := SearchKey(&p, c.key)
		if found {
			t.Errorf("ключ %d не должен находиться", c.key)
		}
		if pos != c.wantPos {
			t.Errorf("ключ %d: позиция %d, ожидали %d", c.key, pos, c.wantPos)
		}
	}
}

// Вставка в случайном порядке — ключи должны остаться отсортированными.
func TestLeafInsertKeepsOrder(t *testing.T) {
	var p Page
	InitLeaf(&p)

	input := []int64{50, 10, 90, 30, 70, 20, 80}
	for _, k := range input {
		if err := LeafInsert(&p, k, []byte(fmt.Sprintf("val-%d", k))); err != nil {
			t.Fatalf("вставка %d: %v", k, err)
		}
	}

	if NumKeys(&p) != uint32(len(input)) {
		t.Fatalf("ключей %d, ожидали %d", NumKeys(&p), len(input))
	}

	// Проверяем сортировку.
	want := []int64{10, 20, 30, 50, 70, 80, 90}
	for i, w := range want {
		if got := KeyAt(&p, uint32(i)); got != w {
			t.Errorf("позиция %d: ключ %d, ожидали %d", i, got, w)
		}
	}

	// Проверяем, что данные не перепутались.
	for i, k := range want {
		data, err := LeafValueAt(&p, uint32(i))
		if err != nil {
			t.Fatal(err)
		}
		expected := []byte(fmt.Sprintf("val-%d", k))
		if !bytes.Equal(data, expected) {
			t.Errorf("ключ %d: данные %q, ожидали %q", k, data, expected)
		}
	}
}

func TestLeafDuplicateKey(t *testing.T) {
	var p Page
	InitLeaf(&p)

	LeafInsert(&p, 42, []byte("first"))

	if err := LeafInsert(&p, 42, []byte("second")); err == nil {
		t.Error("повторная вставка того же ключа должна давать ошибку")
	}
}

func TestLeafFillsUp(t *testing.T) {
	var p Page
	InitLeaf(&p)

	data := make([]byte, 30)
	count := 0

	for i := int64(0); i < 1000; i++ {
		if err := LeafInsert(&p, i, data); err != nil {
			break
		}
		count++
	}

	if count == 0 {
		t.Fatal("не вставилось ни одного ключа")
	}
	if count >= 1000 {
		t.Fatal("лист не заполняется")
	}

	t.Logf("в лист влезло %d записей по 30 байт", count)

	// Всё вставленное должно читаться и быть отсортированным.
	for i := uint32(0); i < NumKeys(&p); i++ {
		if KeyAt(&p, i) != int64(i) {
			t.Fatalf("позиция %d: ключ %d", i, KeyAt(&p, i))
		}
		if _, err := LeafValueAt(&p, i); err != nil {
			t.Fatalf("значение %d не читается: %v", i, err)
		}
	}
}

// Проверяем правило спуска во внутреннем узле.
func TestFindChild(t *testing.T) {
	var p Page
	InitInternal(&p)

	// Разделители: 20, 50. Детей три.
	setKeyAt(&p, 0, 20)
	setKeyAt(&p, 1, 50)
	setNumKeys(&p, 2)

	setChildAt(&p, 0, 100)
	setChildAt(&p, 1, 200)
	setChildAt(&p, 2, 300)

	cases := []struct {
		key   int64
		child uint32
	}{
		{5, 0},  // < 20
		{19, 0}, // < 20
		{20, 1}, // == разделитель → вправо
		{35, 1}, // 20 <= k < 50
		{49, 1},
		{50, 2},  // == разделитель → вправо
		{100, 2}, // >= 50
	}

	for _, c := range cases {
		if got := FindChild(&p, c.key); got != c.child {
			t.Errorf("ключ %d: ребёнок %d, ожидали %d", c.key, got, c.child)
		}
	}
}

func TestNodeTypes(t *testing.T) {
	var leaf, internal Page

	InitLeaf(&leaf)
	InitInternal(&internal)

	if !IsLeaf(&leaf) {
		t.Error("InitLeaf должен давать лист")
	}
	if IsLeaf(&internal) {
		t.Error("InitInternal не должен давать лист")
	}
	if NextLeaf(&leaf) != InvalidPageID {
		t.Error("у нового листа не должно быть следующего")
	}
}

// Узел переживает запись на диск.
func TestNodeSurvivesDisk(t *testing.T) {
	dm, err := NewDiskManager(tempDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer dm.Close()

	var p Page
	InitLeaf(&p)
	LeafInsert(&p, 10, []byte("десять"))
	LeafInsert(&p, 20, []byte("двадцать"))

	id, _ := dm.AllocatePage()
	dm.WritePage(id, &p)

	var back Page
	dm.ReadPage(id, &back)

	if !IsLeaf(&back) {
		t.Fatal("после диска тип узла потерялся")
	}
	if NumKeys(&back) != 2 {
		t.Fatalf("после диска ключей %d", NumKeys(&back))
	}
	if KeyAt(&back, 0) != 10 || KeyAt(&back, 1) != 20 {
		t.Error("ключи после диска неверны")
	}

	data, _ := LeafValueAt(&back, 1)
	if !bytes.Equal(data, []byte("двадцать")) {
		t.Errorf("данные после диска: %q", data)
	}
}
