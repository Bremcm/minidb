package lexer

import (
	"fmt"
	"strings"
)

type TokenType int

const (
	EOF TokenType = iota
	ILLEGAL

	IDENT
	INT
	STRING

	ASTERISK
	EQ
	NEQ
	LT
	GT
	LTE
	GTE

	COMMA
	SEMICOLON
	LPAREN
	RPAREN

	SELECT
	FROM
	WHERE
	INSERT
	INTO
	VALUES
	CREATE
	TABLE
	AND
	OR
)

var tokenNames = map[TokenType]string{
	EOF:       "EOF",
	ILLEGAL:   "ILLEGAL",
	IDENT:     "IDENT",
	INT:       "INT",
	STRING:    "STRING",
	ASTERISK:  "*",
	EQ:        "=",
	NEQ:       "!=",
	LT:        "<",
	GT:        ">",
	LTE:       "<=",
	GTE:       ">=",
	COMMA:     ",",
	SEMICOLON: ";",
	LPAREN:    "(",
	RPAREN:    ")",
	SELECT:    "SELECT",
	FROM:      "FROM",
	WHERE:     "WHERE",
	INSERT:    "INSERT",
	INTO:      "INTO",
	VALUES:    "VALUES",
	CREATE:    "CREATE",
	TABLE:     "TABLE",
	AND:       "AND",
	OR:        "OR",
}

func (t TokenType) String() string {
	if name, ok := tokenNames[t]; ok {
		return name
	}
	return "UNKNOWN"
}

type Token struct {
	Type    TokenType
	Literal string
	Pos     int
}

func (t Token) String() string {
	return fmt.Sprintf("%s(%q)", t.Type, t.Literal)
}

var keywords = map[string]TokenType{
	"select": SELECT,
	"from":   FROM,
	"where":  WHERE,
	"insert": INSERT,
	"into":   INTO,
	"values": VALUES,
	"create": CREATE,
	"table":  TABLE,
	"and":    AND,
	"or":     OR,
}

func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[strings.ToLower(ident)]; ok {
		return tok
	}
	return IDENT
}
