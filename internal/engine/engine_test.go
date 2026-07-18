package engine

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/Bremcm/minidb/internal/lexer"
	"github.com/Bremcm/minidb/internal/parser"
)

// exec — хелпер: прогоняет SQL через всю цепочку.
func exec(t *testing.T, e *Engine, sql string) *Result {
	t.Helper()

	l := lexer.New(sql)
	p := parser.New(l)
	stmt := p.ParseStatement()

	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("%q: ошибки разбора: %v", sql, errs)
	}

	res, err := e.Execute(stmt)
	if err != nil {
		t.Fatalf("%q: ошибка выполнения: %v", sql, err)
	}

	return res
}

// execErr — хелпер: ожидает ОШИБКУ выполнения.
func execErr(t *testing.T, e *Engine, sql string) {
	t.Helper()

	l := lexer.New(sql)
	p := parser.New(l)
	stmt := p.ParseStatement()

	if errs := p.Errors(); len(errs) > 0 {
		return // ошибка на разборе — тоже ошибка, годится
	}

	if _, err := e.Execute(stmt); err == nil {
		t.Errorf("%q: ожидал ошибку, но её нет", sql)
	}
}

// setup — таблица с тремя строками.
func setup(t *testing.T) *Engine {
	t.Helper()

	dir := t.TempDir()
	e, err := New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })

	exec(t, e, "CREATE TABLE users (id INT, name TEXT, age INT)")
	exec(t, e, "INSERT INTO users VALUES (1, 'bob', 30)")
	exec(t, e, "INSERT INTO users VALUES (2, 'alice', 25)")
	exec(t, e, "INSERT INTO users VALUES (3, 'eve', 35)")

	return e
}

func TestSelectAll(t *testing.T) {
	e := setup(t)
	res := exec(t, e, "SELECT * FROM users")

	if len(res.Rows) != 3 {
		t.Fatalf("ожидал 3 строки, получил %d", len(res.Rows))
	}
	if len(res.Columns) != 3 {
		t.Fatalf("ожидал 3 колонки, получил %d", len(res.Columns))
	}
}

func TestWhereFilter(t *testing.T) {
	e := setup(t)
	res := exec(t, e, "SELECT name FROM users WHERE age > 26")

	if len(res.Rows) != 2 {
		t.Fatalf("ожидал 2 строки, получил %d", len(res.Rows))
	}

	names := map[string]bool{}
	for _, row := range res.Rows {
		names[row[0].Text] = true
	}

	if !names["bob"] || !names["eve"] {
		t.Errorf("ожидал bob и eve, получил %v", names)
	}
}

func TestProjection(t *testing.T) {
	e := setup(t)
	res := exec(t, e, "SELECT name FROM users")

	if len(res.Columns) != 1 || res.Columns[0] != "name" {
		t.Fatalf("ожидал одну колонку name, получил %v", res.Columns)
	}
	for _, row := range res.Rows {
		if len(row) != 1 {
			t.Fatalf("в строке должно быть 1 значение, получено %d", len(row))
		}
	}
}

// Главный тест этапа: приоритеты операторов дают РАЗНЫЕ результаты.
func TestPrecedenceAffectsResult(t *testing.T) {
	e := setup(t)

	// AND сильнее: age > 30 OR (name = 'bob' AND age = 30)
	// подходят: eve (35>30), bob (bob и 30)
	res1 := exec(t, e, "SELECT name FROM users WHERE age > 30 OR name = 'bob' AND age = 30")
	if len(res1.Rows) != 2 {
		t.Errorf("без скобок: ожидал 2 строки, получил %d", len(res1.Rows))
	}

	// Скобки перебивают: (age > 30 OR name = 'bob') AND age = 30
	// подходит только bob
	res2 := exec(t, e, "SELECT name FROM users WHERE (age > 30 OR name = 'bob') AND age = 30")
	if len(res2.Rows) != 1 {
		t.Errorf("со скобками: ожидал 1 строку, получил %d", len(res2.Rows))
	}
	if len(res2.Rows) == 1 && res2.Rows[0][0].Text != "bob" {
		t.Errorf("ожидал bob, получил %q", res2.Rows[0][0].Text)
	}
}

func TestTypeChecking(t *testing.T) {
	e := setup(t)

	// TEXT в INT-колонку
	execErr(t, e, "INSERT INTO users VALUES ('x', 'bob', 30)")

	// Не то количество значений
	execErr(t, e, "INSERT INTO users VALUES (1, 'bob')")

	// Сравнение разных типов
	execErr(t, e, "SELECT * FROM users WHERE name > 5")
}

func TestMissingObjects(t *testing.T) {
	e := setup(t)

	execErr(t, e, "SELECT * FROM nosuchtable")
	execErr(t, e, "SELECT nosuchcol FROM users")
	execErr(t, e, "CREATE TABLE users (id INT)") // дубликат
}

func TestStringComparison(t *testing.T) {
	e := setup(t)

	res := exec(t, e, "SELECT name FROM users WHERE name = 'alice'")
	if len(res.Rows) != 1 {
		t.Fatalf("ожидал 1 строку, получил %d", len(res.Rows))
	}
	if res.Rows[0][0].Text != "alice" {
		t.Errorf("ожидал alice, получил %q", res.Rows[0][0].Text)
	}

	// Лексикографическое сравнение: alice < bob < eve
	res = exec(t, e, "SELECT name FROM users WHERE name < 'bob'")
	if len(res.Rows) != 1 || res.Rows[0][0].Text != "alice" {
		t.Errorf("name < 'bob': ожидал только alice, получил %v", res.Rows)
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persist.db")

	// Сессия 1: создаём и заполняем.
	e1, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	exec(t, e1, "CREATE TABLE users (id INT, name TEXT, age INT)")
	exec(t, e1, "INSERT INTO users VALUES (1, 'bob', 30)")
	exec(t, e1, "INSERT INTO users VALUES (2, 'alice', 25)")
	if err := e1.Close(); err != nil {
		t.Fatal(err)
	}

	// Сессия 2: открываем заново.
	e2, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()

	res := exec(t, e2, "SELECT * FROM users")
	if len(res.Rows) != 2 {
		t.Fatalf("после перезапуска ожидали 2 строки, получили %d", len(res.Rows))
	}

	res = exec(t, e2, "SELECT name FROM users WHERE age > 26")
	if len(res.Rows) != 1 || res.Rows[0][0].Text != "bob" {
		t.Errorf("фильтр после перезапуска: %v", res.Rows)
	}
}

// Много строк — проверяем, что данные переливаются на вторую страницу.
func TestMultiplePages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "many.db")

	e, err := New(path)
	if err != nil {
		t.Fatal(err)
	}

	exec(t, e, "CREATE TABLE t (id INT, data TEXT)")

	const n = 200
	for i := 0; i < n; i++ {
		exec(t, e, fmt.Sprintf(
			"INSERT INTO t VALUES (%d, 'некоторый текст для объёма %d')", i, i))
	}
	e.Close()

	// Переоткрываем и считаем.
	e2, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer e2.Close()

	res := exec(t, e2, "SELECT * FROM t")
	if len(res.Rows) != n {
		t.Fatalf("ожидали %d строк, получили %d", n, len(res.Rows))
	}
}
