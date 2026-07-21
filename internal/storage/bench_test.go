package storage

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/Bremcm/minidb/internal/storage/disk"
)

// buildBigTree создаёт дерево с n записями.
func buildBigTree(tb testing.TB, n int) *BTree {
	tb.Helper()

	path := filepath.Join(tb.TempDir(), "bench.db")
	dm, err := disk.NewDiskManager(path)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { dm.Close() })

	pager := disk.NewPager(dm)
	bt, err := NewBTree(pager)
	if err != nil {
		tb.Fatal(err)
	}

	for i := 0; i < n; i++ {
		data := []byte(fmt.Sprintf("row-%d-with-some-payload", i))
		if err := bt.Insert(int64(i), data); err != nil {
			tb.Fatal(err)
		}
	}

	return bt
}

// Поиск по индексу: сколько стоит найти одну строку в большом дереве.
func BenchmarkIndexSearch(b *testing.B) {
	const n = 1_000_000
	bt := buildBigTree(b, n)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := int64(n - 1 - (i % 1000))
		_, found, err := bt.Search(key)
		if err != nil {
			b.Fatal(err)
		}
		if !found {
			b.Fatalf("ключ %d не найден", key)
		}
	}
}

// Полный перебор: имитируем поиск строки сканированием всего дерева.
func BenchmarkFullScan(b *testing.B) {
	const n = 1_000_000
	bt := buildBigTree(b, n)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Ищем ключи ближе к концу — заставляем обойти почти всё дерево.
		target := int64(n - 1 - (i % 1000))
		var foundData []byte

		bt.ScanAll(func(key int64, data []byte) error {
			if key == target {
				foundData = data
				return errStop
			}
			return nil
		})

		if foundData == nil {
			b.Fatalf("ключ %d не найден", target)
		}
	}
}

var errStop = fmt.Errorf("stop")
