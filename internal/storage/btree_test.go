package storage

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/Bremcm/minidb/internal/storage/disk"
)

// newTestTree создаёт дерево на временном файле.
func newTestTree(t *testing.T) (*BTree, *disk.Pager) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "btree.db")
	dm, err := disk.NewDiskManager(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dm.Close() })

	pager := disk.NewPager(dm)

	bt, err := NewBTree(pager)
	if err != nil {
		t.Fatal(err)
	}

	return bt, pager
}

func TestSearchEmpty(t *testing.T) {
	bt, _ := newTestTree(t)

	_, found, err := bt.Search(42)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("в пустом дереве ничего не должно находиться")
	}
}

func TestInsertAndSearch(t *testing.T) {
	bt, _ := newTestTree(t)

	keys := []int64{50, 10, 90, 30, 70}
	for _, k := range keys {
		data := []byte(fmt.Sprintf("значение-%d", k))
		if err := bt.Insert(k, data); err != nil {
			t.Fatalf("вставка %d: %v", k, err)
		}
	}

	// Каждый ключ должен находиться.
	for _, k := range keys {
		data, found, err := bt.Search(k)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Errorf("ключ %d не найден", k)
			continue
		}

		want := []byte(fmt.Sprintf("значение-%d", k))
		if !bytes.Equal(data, want) {
			t.Errorf("ключ %d: данные %q, ожидали %q", k, data, want)
		}
	}

	// Отсутствующие ключи не должны находиться.
	for _, k := range []int64{5, 20, 100} {
		if _, found, _ := bt.Search(k); found {
			t.Errorf("ключ %d не должен находиться", k)
		}
	}
}

// Сканирование должно идти строго по возрастанию ключа.
func TestScanAllOrder(t *testing.T) {
	bt, _ := newTestTree(t)

	input := []int64{50, 10, 90, 30, 70, 20}
	for _, k := range input {
		bt.Insert(k, []byte(fmt.Sprintf("v%d", k)))
	}

	var got []int64
	err := bt.ScanAll(func(key int64, data []byte) error {
		got = append(got, key)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []int64{10, 20, 30, 50, 70, 90}
	if len(got) != len(want) {
		t.Fatalf("получили %d ключей, ожидали %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("позиция %d: ключ %d, ожидали %d", i, got[i], want[i])
		}
	}
}

// Сканирование с середины: только ключи >= startKey.
func TestScanFromKey(t *testing.T) {
	bt, _ := newTestTree(t)

	for _, k := range []int64{10, 20, 30, 40, 50} {
		bt.Insert(k, []byte("x"))
	}

	cases := []struct {
		start int64
		want  []int64
	}{
		{0, []int64{10, 20, 30, 40, 50}}, // до всех
		{25, []int64{30, 40, 50}},        // между ключами
		{30, []int64{30, 40, 50}},        // точное совпадение включается
		{50, []int64{50}},                // последний
		{100, nil},                       // после всех
	}

	for _, c := range cases {
		var got []int64
		bt.Scan(c.start, func(key int64, data []byte) error {
			got = append(got, key)
			return nil
		})

		if len(got) != len(c.want) {
			t.Errorf("start=%d: получили %v, ожидали %v", c.start, got, c.want)
			continue
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Errorf("start=%d позиция %d: %d, ожидали %d",
					c.start, i, got[i], c.want[i])
			}
		}
	}
}

// Колбэк может прервать обход.
func TestScanEarlyStop(t *testing.T) {
	bt, _ := newTestTree(t)

	for k := int64(1); k <= 10; k++ {
		bt.Insert(k, []byte("x"))
	}

	stopErr := fmt.Errorf("хватит")
	count := 0

	err := bt.ScanAll(func(key int64, data []byte) error {
		count++
		if count == 3 {
			return stopErr
		}
		return nil
	})

	if err != stopErr {
		t.Errorf("ожидали stopErr, получили %v", err)
	}
	if count != 3 {
		t.Errorf("обработано %d записей, ожидали 3", count)
	}
}

// Дерево переживает переоткрытие файла.
func TestTreePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persist.db")

	var rootID disk.PageID

	// Сессия 1.
	{
		dm, err := disk.NewDiskManager(path)
		if err != nil {
			t.Fatal(err)
		}
		pager := disk.NewPager(dm)

		bt, err := NewBTree(pager)
		if err != nil {
			t.Fatal(err)
		}
		rootID = bt.Root()

		for _, k := range []int64{10, 20, 30} {
			bt.Insert(k, []byte(fmt.Sprintf("v%d", k)))
		}

		if err := pager.Close(); err != nil {
			t.Fatal(err)
		}
	}

	// Сессия 2.
	{
		dm, err := disk.NewDiskManager(path)
		if err != nil {
			t.Fatal(err)
		}
		defer dm.Close()

		pager := disk.NewPager(dm)
		bt := OpenBTree(pager, rootID)

		data, found, err := bt.Search(20)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatal("ключ 20 не найден после переоткрытия")
		}
		if !bytes.Equal(data, []byte("v20")) {
			t.Errorf("данные после переоткрытия: %q", data)
		}
	}
}

// Спуск по многоуровневому дереву, собранному вручную.
func TestMultiLevelDescent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multi.db")
	dm, err := disk.NewDiskManager(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dm.Close()

	pager := disk.NewPager(dm)

	// Три листа.
	leafIDs := make([]disk.PageID, 3)
	leafKeys := [][]int64{
		{10, 20},
		{30, 40},
		{50, 60},
	}

	for i := range leafIDs {
		id, page, err := pager.AllocatePage()
		if err != nil {
			t.Fatal(err)
		}
		disk.InitLeaf(page)
		for _, k := range leafKeys[i] {
			disk.LeafInsert(page, k, []byte(fmt.Sprintf("v%d", k)))
		}
		pager.MarkDirty(id)
		leafIDs[i] = id
	}

	// Связываем листья в цепочку.
	for i := 0; i < len(leafIDs)-1; i++ {
		page, _ := pager.FetchPage(leafIDs[i])
		disk.SetNextLeaf(page, leafIDs[i+1])
		pager.MarkDirty(leafIDs[i])
	}

	// Корень: разделители 30 и 50.
	rootID, rootPage, err := pager.AllocatePage()
	if err != nil {
		t.Fatal(err)
	}
	disk.InitInternal(rootPage)
	disk.SetKeyForTest(rootPage, 0, 30)
	disk.SetKeyForTest(rootPage, 1, 50)
	disk.SetNumKeysForTest(rootPage, 2)
	disk.SetChildForTest(rootPage, 0, leafIDs[0])
	disk.SetChildForTest(rootPage, 1, leafIDs[1])
	disk.SetChildForTest(rootPage, 2, leafIDs[2])
	pager.MarkDirty(rootID)

	bt := OpenBTree(pager, rootID)

	// Каждый ключ должен находиться через спуск.
	for _, keys := range leafKeys {
		for _, k := range keys {
			data, found, err := bt.Search(k)
			if err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Errorf("ключ %d не найден в многоуровневом дереве", k)
				continue
			}
			want := []byte(fmt.Sprintf("v%d", k))
			if !bytes.Equal(data, want) {
				t.Errorf("ключ %d: данные %q", k, data)
			}
		}
	}

	// Сканирование должно пройти все листья по цепочке.
	var got []int64
	bt.ScanAll(func(key int64, data []byte) error {
		got = append(got, key)
		return nil
	})

	want := []int64{10, 20, 30, 40, 50, 60}
	if len(got) != len(want) {
		t.Fatalf("сканирование дало %v, ожидали %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("позиция %d: %d, ожидали %d", i, got[i], want[i])
		}
	}
}

// Вставка большого количества ключей должна вырастить дерево.
func TestInsertManySequential(t *testing.T) {
	bt, _ := newTestTree(t)

	const n = 5000

	for i := int64(0); i < n; i++ {
		data := []byte(fmt.Sprintf("значение номер %d", i))
		if err := bt.Insert(i, data); err != nil {
			t.Fatalf("вставка %d: %v", i, err)
		}
	}

	// Все ключи должны находиться.
	for i := int64(0); i < n; i++ {
		data, found, err := bt.Search(i)
		if err != nil {
			t.Fatalf("поиск %d: %v", i, err)
		}
		if !found {
			t.Fatalf("ключ %d не найден после %d вставок", i, n)
		}
		want := []byte(fmt.Sprintf("значение номер %d", i))
		if !bytes.Equal(data, want) {
			t.Fatalf("ключ %d: данные %q, ожидали %q", i, data, want)
		}
	}
}

// То же, но ключи вставляются вразнобой.
func TestInsertManyShuffled(t *testing.T) {
	bt, _ := newTestTree(t)

	const n = 3000

	// Псевдослучайный порядок без импорта math/rand:
	// шаг взаимно прост с n, значит обойдём все значения.
	keys := make([]int64, 0, n)
	for i, k := 0, int64(0); i < n; i++ {
		keys = append(keys, k)
		k = (k + 997) % n
	}

	for _, k := range keys {
		if err := bt.Insert(k, []byte(fmt.Sprintf("v%d", k))); err != nil {
			t.Fatalf("вставка %d: %v", k, err)
		}
	}

	for i := int64(0); i < n; i++ {
		_, found, err := bt.Search(i)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatalf("ключ %d не найден", i)
		}
	}
}

// Главный тест: после множества сплитов сканирование
// должно дать все ключи строго по возрастанию.
func TestScanAfterSplits(t *testing.T) {
	bt, _ := newTestTree(t)

	const n = 2000

	for i := int64(n - 1); i >= 0; i-- { // вставляем в обратном порядке
		bt.Insert(i, []byte(fmt.Sprintf("v%d", i)))
	}

	var prev int64 = -1
	count := 0

	err := bt.ScanAll(func(key int64, data []byte) error {
		if key <= prev {
			return fmt.Errorf("порядок нарушен: %d после %d", key, prev)
		}
		prev = key
		count++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if count != n {
		t.Fatalf("просканировано %d записей, ожидали %d", count, n)
	}
}

// Диапазонное сканирование после сплитов.
func TestRangeScanAfterSplits(t *testing.T) {
	bt, _ := newTestTree(t)

	const n = 2000
	for i := int64(0); i < n; i++ {
		bt.Insert(i, []byte("x"))
	}

	const start = 1500
	count := 0

	bt.Scan(start, func(key int64, data []byte) error {
		if key < start {
			return fmt.Errorf("ключ %d меньше стартового %d", key, start)
		}
		count++
		return nil
	})

	if count != n-start {
		t.Fatalf("в диапазоне %d записей, ожидали %d", count, n-start)
	}
}

// Дубликаты по-прежнему отвергаются.
func TestInsertDuplicateAfterSplit(t *testing.T) {
	bt, _ := newTestTree(t)

	for i := int64(0); i < 500; i++ {
		bt.Insert(i, []byte("x"))
	}

	if err := bt.Insert(250, []byte("другое")); err == nil {
		t.Error("повторная вставка ключа 250 должна давать ошибку")
	}
}

// Дерево с несколькими уровнями переживает переоткрытие.
func TestLargeTreePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.db")

	var rootID disk.PageID
	const n = 3000

	{
		dm, err := disk.NewDiskManager(path)
		if err != nil {
			t.Fatal(err)
		}
		pager := disk.NewPager(dm)

		bt, err := NewBTree(pager)
		if err != nil {
			t.Fatal(err)
		}

		for i := int64(0); i < n; i++ {
			if err := bt.Insert(i, []byte(fmt.Sprintf("v%d", i))); err != nil {
				t.Fatal(err)
			}
		}

		rootID = bt.Root()
		if err := pager.Close(); err != nil {
			t.Fatal(err)
		}
	}

	{
		dm, err := disk.NewDiskManager(path)
		if err != nil {
			t.Fatal(err)
		}
		defer dm.Close()

		pager := disk.NewPager(dm)
		bt := OpenBTree(pager, rootID)

		count := 0
		bt.ScanAll(func(key int64, data []byte) error {
			count++
			return nil
		})

		if count != n {
			t.Fatalf("после переоткрытия %d записей, ожидали %d", count, n)
		}

		// Точечный поиск в середине.
		data, found, _ := bt.Search(1500)
		if !found {
			t.Fatal("ключ 1500 не найден после переоткрытия")
		}
		if !bytes.Equal(data, []byte("v1500")) {
			t.Errorf("данные: %q", data)
		}
	}
}
