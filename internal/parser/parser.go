package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Bremcm/minidb/internal/ast"
	"github.com/Bremcm/minidb/internal/lexer"
)

type Parser struct {
	l *lexer.Lexer

	curToken  lexer.Token
	peekToken lexer.Token

	errors []string
}

func New(l *lexer.Lexer) *Parser {
	p := &Parser{
		l:      l,
		errors: []string{},
	}

	p.nextToken()
	p.nextToken()

	return p
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

func (p *Parser) curTokenIs(t lexer.TokenType) bool {
	return p.curToken.Type == t
}

func (p *Parser) peekTokenIs(t lexer.TokenType) bool {
	return p.peekToken.Type == t
}

func (p *Parser) expectPeek(t lexer.TokenType) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}
	p.peekError(t)
	return false
}

func (p *Parser) peekError(t lexer.TokenType) {
	msg := fmt.Sprintf("ожидал %s, получил %s (позиция %d)",
		t, p.peekToken.Type, p.peekToken.Pos)
	p.errors = append(p.errors, msg)
}

func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) ParseStatement() ast.Statement {
	switch p.curToken.Type {
	case lexer.SELECT:
		return p.parseSelect()
	case lexer.INSERT:
		return p.parseInsert()
	case lexer.CREATE:
		return p.parseCreateTable()
	default:
		p.errors = append(p.errors,
			fmt.Sprintf("неизвестная команда: %s", p.curToken.Literal))
		return nil
	}
}

func (p *Parser) parseSelect() *ast.SelectStatement {
	stmt := &ast.SelectStatement{}

	if p.peekTokenIs(lexer.ASTERISK) {
		p.nextToken()
		stmt.Columns = nil
	} else {
		stmt.Columns = p.parseColumnList()
		if stmt.Columns == nil {
			return nil
		}
	}

	if !p.expectPeek(lexer.FROM) {
		return nil
	}
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	stmt.From = p.curToken.Literal

	if p.peekTokenIs(lexer.WHERE) {
		p.nextToken()
		p.nextToken()

		stmt.Where = p.parseExpression(LOWEST)
		if stmt.Where == nil {
			return nil
		}
	}

	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseColumnList() []ast.Expression {
	cols := []ast.Expression{}

	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	cols = append(cols, &ast.Identifier{Name: p.curToken.Literal})

	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken()
		if !p.expectPeek(lexer.IDENT) {
			return nil
		}
		cols = append(cols, &ast.Identifier{Name: p.curToken.Literal})
	}

	return cols
}

const (
	LOWEST int = iota
	OR_PREC
	AND_PREC
	COMPARISON
)

var precedences = map[lexer.TokenType]int{
	lexer.OR:  OR_PREC,
	lexer.AND: AND_PREC,

	lexer.EQ:  COMPARISON,
	lexer.NEQ: COMPARISON,
	lexer.LT:  COMPARISON,
	lexer.GT:  COMPARISON,
	lexer.LTE: COMPARISON,
	lexer.GTE: COMPARISON,
}

func (p *Parser) peekPrecedence() int {
	if prec, ok := precedences[p.peekToken.Type]; ok {
		return prec
	}
	return LOWEST
}

func (p *Parser) parseExpression(precedence int) ast.Expression {
	left := p.parsePrefix()
	if left == nil {
		return nil
	}

	for !p.peekTokenIs(lexer.SEMICOLON) && precedence < p.peekPrecedence() {
		if _, isOperator := precedences[p.peekToken.Type]; !isOperator {
			return left
		}

		p.nextToken()
		left = p.parseInfix(left)
		if left == nil {
			return nil
		}
	}

	return left
}

func (p *Parser) parsePrefix() ast.Expression {
	switch p.curToken.Type {

	case lexer.IDENT:
		return &ast.Identifier{Name: p.curToken.Literal}

	case lexer.INT:
		val, err := strconv.ParseInt(p.curToken.Literal, 10, 64)
		if err != nil {
			p.errors = append(p.errors,
				fmt.Sprintf("не могу разобрать число %q", p.curToken.Literal))
			return nil
		}
		return &ast.IntegerLiteral{Value: val}

	case lexer.STRING:
		return &ast.StringLiteral{Value: p.curToken.Literal}

	case lexer.LPAREN:
		p.nextToken()
		expr := p.parseExpression(LOWEST)
		if expr == nil {
			return nil
		}
		if !p.expectPeek(lexer.RPAREN) {
			return nil
		}
		return expr

	default:
		p.errors = append(p.errors,
			fmt.Sprintf("неожиданный токен в выражении: %s", p.curToken))
		return nil
	}
}

func (p *Parser) parseInfix(left ast.Expression) ast.Expression {
	expr := &ast.BinaryExpression{
		Left:     left,
		Operator: p.curToken.Type,
	}

	precedence := precedences[p.curToken.Type]
	p.nextToken()

	expr.Right = p.parseExpression(precedence)
	if expr.Right == nil {
		return nil
	}

	return expr
}

func (p *Parser) parseInsert() *ast.InsertStatement {
	stmt := &ast.InsertStatement{}

	if !p.expectPeek(lexer.INTO) {
		return nil
	}
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	stmt.Table = p.curToken.Literal

	if !p.expectPeek(lexer.VALUES) {
		return nil
	}
	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	stmt.Values = []ast.Expression{}

	p.nextToken()
	val := p.parsePrefix()
	if val == nil {
		return nil
	}
	stmt.Values = append(stmt.Values, val)

	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken()
		p.nextToken()

		val := p.parsePrefix()
		if val == nil {
			return nil
		}
		stmt.Values = append(stmt.Values, val)
	}

	if !p.expectPeek(lexer.RPAREN) {
		return nil
	}

	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseCreateTable() *ast.CreateTableStatement {
	stmt := &ast.CreateTableStatement{}

	if !p.expectPeek(lexer.TABLE) {
		return nil
	}
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	stmt.Table = p.curToken.Literal

	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	stmt.Columns = []ast.ColumnDef{}

	col := p.parseColumnDef()
	if col == nil {
		return nil
	}
	stmt.Columns = append(stmt.Columns, *col)

	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken()

		col := p.parseColumnDef()
		if col == nil {
			return nil
		}
		stmt.Columns = append(stmt.Columns, *col)
	}

	if !p.expectPeek(lexer.RPAREN) {
		return nil
	}

	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseColumnDef() *ast.ColumnDef {
	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	name := p.curToken.Literal

	if !p.expectPeek(lexer.IDENT) {
		return nil
	}
	typ := strings.ToUpper(p.curToken.Literal)

	if typ != "INT" && typ != "TEXT" {
		p.errors = append(p.errors,
			fmt.Sprintf("неизвестный тип %q (поддерживаются INT и TEXT)", typ))
		return nil
	}

	return &ast.ColumnDef{Name: name, Type: typ}
}
