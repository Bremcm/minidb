package parser

import (
	"testing"

	"github.com/Bremcm/minidb/internal/ast"
	"github.com/Bremcm/minidb/internal/lexer"
)

// parse — хелпер: прогоняет вход через лексер и парсер, валит тест при ошибках.
func parse(t *testing.T, input string) ast.Statement {
	t.Helper()

	l := lexer.New(input)
	p := New(l)
	stmt := p.ParseStatement()

	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("ошибки парсера: %v", errs)
	}
	if stmt == nil {
		t.Fatal("ParseStatement вернул nil")
	}

	return stmt
}

func TestSelectSimple(t *testing.T) {
	stmt := parse(t, "SELECT name, age FROM users;")

	sel, ok := stmt.(*ast.SelectStatement)
	if !ok {
		t.Fatalf("ожидал *ast.SelectStatement, получил %T", stmt)
	}

	if sel.From != "users" {
		t.Errorf("From: ожидал users, получил %q", sel.From)
	}
	if len(sel.Columns) != 2 {
		t.Fatalf("ожидал 2 колонки, получил %d", len(sel.Columns))
	}
	if sel.Where != nil {
		t.Errorf("Where должен быть nil, получил %v", sel.Where)
	}
}

func TestSelectStar(t *testing.T) {
	stmt := parse(t, "SELECT * FROM users")
	sel := stmt.(*ast.SelectStatement)

	if sel.Columns != nil {
		t.Errorf("SELECT * должен дать Columns == nil, получил %v", sel.Columns)
	}
}

// Главный тест этапа: проверяет, что приоритеты операторов работают.
// AND слабее сравнения, поэтому наверху дерева ДОЛЖЕН быть AND.
func TestOperatorPrecedence(t *testing.T) {
	stmt := parse(t, "SELECT * FROM t WHERE age > 25 AND city = 'Kraków'")
	sel := stmt.(*ast.SelectStatement)

	root, ok := sel.Where.(*ast.BinaryExpression)
	if !ok {
		t.Fatalf("корень Where: ожидал BinaryExpression, получил %T", sel.Where)
	}

	if root.Operator != lexer.AND {
		t.Fatalf("корень должен быть AND, получил %s", root.Operator)
	}

	// Слева должно быть age > 25
	left, ok := root.Left.(*ast.BinaryExpression)
	if !ok {
		t.Fatalf("левое поддерево: ожидал BinaryExpression, получил %T", root.Left)
	}
	if left.Operator != lexer.GT {
		t.Errorf("левый оператор: ожидал GT, получил %s", left.Operator)
	}

	// Справа должно быть city = 'Kraków'
	right, ok := root.Right.(*ast.BinaryExpression)
	if !ok {
		t.Fatalf("правое поддерево: ожидал BinaryExpression, получил %T", root.Right)
	}
	if right.Operator != lexer.EQ {
		t.Errorf("правый оператор: ожидал EQ, получил %s", right.Operator)
	}
}

// Скобки должны ПЕРЕБИВАТЬ приоритет: OR слабее AND,
// но в скобках он схватит первым, и наверху окажется AND.
func TestParentheses(t *testing.T) {
	stmt := parse(t, "SELECT * FROM t WHERE (a = 1 OR b = 2) AND c = 3")
	sel := stmt.(*ast.SelectStatement)

	root := sel.Where.(*ast.BinaryExpression)
	if root.Operator != lexer.AND {
		t.Fatalf("корень должен быть AND, получил %s", root.Operator)
	}

	left := root.Left.(*ast.BinaryExpression)
	if left.Operator != lexer.OR {
		t.Errorf("в скобках должен быть OR, получил %s", left.Operator)
	}
}

func TestInsert(t *testing.T) {
	stmt := parse(t, "INSERT INTO users VALUES (1, 'bob', 30)")

	ins, ok := stmt.(*ast.InsertStatement)
	if !ok {
		t.Fatalf("ожидал *ast.InsertStatement, получил %T", stmt)
	}

	if ins.Table != "users" {
		t.Errorf("Table: ожидал users, получил %q", ins.Table)
	}
	if len(ins.Values) != 3 {
		t.Fatalf("ожидал 3 значения, получил %d", len(ins.Values))
	}

	// Первое значение — число 1
	num, ok := ins.Values[0].(*ast.IntegerLiteral)
	if !ok {
		t.Fatalf("values[0]: ожидал IntegerLiteral, получил %T", ins.Values[0])
	}
	if num.Value != 1 {
		t.Errorf("values[0]: ожидал 1, получил %d", num.Value)
	}

	// Второе — строка
	str, ok := ins.Values[1].(*ast.StringLiteral)
	if !ok {
		t.Fatalf("values[1]: ожидал StringLiteral, получил %T", ins.Values[1])
	}
	if str.Value != "bob" {
		t.Errorf("values[1]: ожидал bob, получил %q", str.Value)
	}
}

func TestCreateTable(t *testing.T) {
	stmt := parse(t, "CREATE TABLE users (id INT, name TEXT, age INT)")

	ct, ok := stmt.(*ast.CreateTableStatement)
	if !ok {
		t.Fatalf("ожидал *ast.CreateTableStatement, получил %T", stmt)
	}

	if len(ct.Columns) != 3 {
		t.Fatalf("ожидал 3 колонки, получил %d", len(ct.Columns))
	}
	if ct.Columns[0].Name != "id" || ct.Columns[0].Type != "INT" {
		t.Errorf("колонка 0: получил %+v", ct.Columns[0])
	}
	if ct.Columns[1].Name != "name" || ct.Columns[1].Type != "TEXT" {
		t.Errorf("колонка 1: получил %+v", ct.Columns[1])
	}
}

func TestErrors(t *testing.T) {
	cases := []string{
		"SELECT name users",         // нет FROM
		"SELECT FROM users",         // нет колонок
		"INSERT users VALUES (1)",   // нет INTO
		"CREATE TABLE t (id FLOAT)", // неподдерживаемый тип
	}

	for _, input := range cases {
		l := lexer.New(input)
		p := New(l)
		p.ParseStatement()

		if len(p.Errors()) == 0 {
			t.Errorf("вход %q: ожидал ошибку, но её нет", input)
		}
	}
}
