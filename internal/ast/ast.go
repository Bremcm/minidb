package ast

import "github.com/Bremcm/minidb/internal/lexer"

type Node interface {
	node()
}

type Statement interface {
	Node
	statement()
}

type Expression interface {
	Node
	expression()
}

type Identifier struct {
	Name string
}

func (i *Identifier) node()       {}
func (i *Identifier) expression() {}

type IntegerLiteral struct {
	Value int64
}

func (il *IntegerLiteral) node()       {}
func (il *IntegerLiteral) expression() {}

type StringLiteral struct {
	Value string
}

func (sl *StringLiteral) node()       {}
func (sl *StringLiteral) expression() {}

type BinaryExpression struct {
	Left     Expression
	Operator lexer.TokenType
	Right    Expression
}

func (be *BinaryExpression) node()       {}
func (be *BinaryExpression) expression() {}

type SelectStatement struct {
	Columns []Expression
	From    string
	Where   Expression
}

func (ss *SelectStatement) node()      {}
func (ss *SelectStatement) statement() {}

type InsertStatement struct {
	Table  string
	Values []Expression
}

func (is *InsertStatement) node()      {}
func (is *InsertStatement) statement() {}

type ColumnDef struct {
	Name string
	Type string
}

type CreateTableStatement struct {
	Table   string
	Columns []ColumnDef
}

func (cts *CreateTableStatement) node()      {}
func (cts *CreateTableStatement) statement() {}
