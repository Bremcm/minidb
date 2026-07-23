package main

import (
	"fmt"
	"os"

	"github.com/Bremcm/minidb/internal/storage"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "использование: crashtest <db> <режим>")
		fmt.Fprintln(os.Stderr, "режимы: fill | verify")
		os.Exit(1)
	}

	dbPath := os.Args[1]
	mode := os.Args[2]

	cat, err := storage.OpenCatalog(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "открытие: %v\n", err)
		os.Exit(1)
	}

	switch mode {
	case "fill":
		fill(cat)
	case "verify":
		verify(cat, dbPath)
	default:
		fmt.Fprintf(os.Stderr, "неизвестный режим %q\n", mode)
		os.Exit(1)
	}
}

func fill(cat *storage.Catalog) {
	if _, err := cat.GetTable("t"); err != nil {
		cols := []storage.Column{
			{Name: "id", Type: storage.TypeInt},
			{Name: "data", Type: storage.TypeText},
		}
		if err := cat.CreateTable("t", cols); err != nil {
			fmt.Fprintf(os.Stderr, "создание таблицы: %v\n", err)
			os.Exit(1)
		}
	}

	tbl, _ := cat.GetTable("t")

	maxID := int64(0)
	tbl.ScanRows(func(row storage.Row) error {
		if row[0].Int > maxID {
			maxID = row[0].Int
		}
		return nil
	})

	fmt.Printf("начинаем с id %d\n", maxID+1)

	for i := maxID + 1; ; i++ {
		row := storage.Row{
			storage.NewInt(i),
			storage.NewText(fmt.Sprintf("запись номер %d с текстом", i)),
		}
		if err := tbl.AppendRow(row); err != nil {
			fmt.Fprintf(os.Stderr, "вставка %d: %v\n", i, err)
			os.Exit(1)
		}

		// Сохраняем каждые 100 вставок.
		if i%100 == 0 {
			if err := cat.Save(); err != nil {
				fmt.Fprintf(os.Stderr, "сохранение: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("сохранено до id %d\n", i)
		}
	}
}

func verify(cat *storage.Catalog, dbPath string) {
	tbl, err := cat.GetTable("t")
	if err != nil {
		fmt.Fprintf(os.Stderr, "таблица не найдена: %v\n", err)
		os.Exit(1)
	}

	// Проверяем целостность: ключи строго возрастают, без дыр и дублей.
	var prev int64 = 0
	count := 0
	broken := false

	err = tbl.ScanRows(func(row storage.Row) error {
		id := row[0].Int

		if id != prev+1 {
			fmt.Printf("ДЫРА или НЕПОРЯДОК: после %d идёт %d\n", prev, id)
			broken = true
		}
		prev = id
		count++
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ошибка сканирования: %v\n", err)
		os.Exit(1)
	}

	for i := int64(1); i <= prev; i++ {
		_, found, err := tbl.SearchRow(i)
		if err != nil {
			fmt.Fprintf(os.Stderr, "поиск %d: %v\n", i, err)
			os.Exit(1)
		}
		if !found {
			fmt.Printf("ПОТЕРЯН ключ %d (есть в скане, нет в поиске)\n", i)
			broken = true
		}
	}

	cat.Close()

	if broken {
		fmt.Printf("ДЕРЕВО ПОВРЕЖДЕНО: %d строк, но есть нарушения\n", count)
		os.Exit(1)
	}

	fmt.Printf("OK: %d строк, все ключи 1..%d на месте, дерево согласовано\n",
		count, prev)
}
