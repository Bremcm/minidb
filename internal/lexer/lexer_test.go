package lexer

import "testing"

func TestNextToken(t *testing.T) {
	input := `SELECT name, age FROM users WHERE age >= 25 AND city != 'Kraków';`

	expected := []struct {
		typ TokenType
		lit string
	}{
		{SELECT, "SELECT"},
		{IDENT, "name"},
		{COMMA, ","},
		{IDENT, "age"},
		{FROM, "FROM"},
		{IDENT, "users"},
		{WHERE, "WHERE"},
		{IDENT, "age"},
		{GTE, ">="},
		{INT, "25"},
		{AND, "AND"},
		{IDENT, "city"},
		{NEQ, "!="},
		{STRING, "Kraków"},
		{SEMICOLON, ";"},
		{EOF, ""},
	}

	l := New(input)

	for i, exp := range expected {
		tok := l.NextToken()

		if tok.Type != exp.typ {
			t.Fatalf("test[%d]: тип — ожидал %s, получил %s (literal=%q)",
				i, exp.typ, tok.Type, tok.Literal)
		}
		if tok.Literal != exp.lit {
			t.Fatalf("test[%d]: literal — ожидал %q, получил %q",
				i, exp.lit, tok.Literal)
		}
	}
}

func TestNoWhitespace(t *testing.T) {
	l := New("age>25")

	expected := []TokenType{IDENT, GT, INT, EOF}

	for i, want := range expected {
		tok := l.NextToken()
		if tok.Type != want {
			t.Fatalf("test[%d]: ожидал %s, получил %s", i, want, tok.Type)
		}
	}
}

func TestUnterminatedString(t *testing.T) {
	l := New("SELECT 'abc")

	l.NextToken()
	tok := l.NextToken()

	if tok.Type != STRING || tok.Literal != "abc" {
		t.Fatalf("незакрытая кавычка: ожидал STRING(\"abc\"), получил %s", tok)
	}
}
