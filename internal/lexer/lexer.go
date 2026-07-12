package lexer

type Lexer struct {
	input   string
	pos     int
	readPos int
	ch      byte
}

func New(input string) *Lexer {
	l := &Lexer{input: input}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.readPos >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPos]
	}
	l.pos = l.readPos
	l.readPos++
}

func (l *Lexer) peekChar() byte {
	if l.readPos >= len(l.input) {
		return 0
	}
	return l.input[l.readPos]
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		l.readChar()
	}
}

func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	pos := l.pos
	var tok Token

	switch l.ch {
	case 0:
		tok = Token{Type: EOF, Literal: "", Pos: pos}

	case '*':
		tok = Token{Type: ASTERISK, Literal: "*", Pos: pos}
	case ',':
		tok = Token{Type: COMMA, Literal: ",", Pos: pos}
	case ';':
		tok = Token{Type: SEMICOLON, Literal: ";", Pos: pos}
	case '(':
		tok = Token{Type: LPAREN, Literal: "(", Pos: pos}
	case ')':
		tok = Token{Type: RPAREN, Literal: ")", Pos: pos}
	case '=':
		tok = Token{Type: EQ, Literal: "=", Pos: pos}

	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: LTE, Literal: "<=", Pos: pos}
		} else if l.peekChar() == '>' {
			l.readChar()
			tok = Token{Type: NEQ, Literal: "<>", Pos: pos}
		} else {
			tok = Token{Type: LT, Literal: "<", Pos: pos}
		}

	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: GTE, Literal: ">=", Pos: pos}
		} else {
			tok = Token{Type: GT, Literal: ">", Pos: pos}
		}

	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: NEQ, Literal: "!=", Pos: pos}
		} else {
			tok = Token{Type: ILLEGAL, Literal: string(l.ch), Pos: pos}
		}

	case '\'':
		lit := l.readString()
		return Token{Type: STRING, Literal: lit, Pos: pos}

	default:
		if isLetter(l.ch) {
			lit := l.readIdentifier()
			return Token{Type: LookupIdent(lit), Literal: lit, Pos: pos}
		}
		if isDigit(l.ch) {
			lit := l.readNumber()
			return Token{Type: INT, Literal: lit, Pos: pos}
		}
		tok = Token{Type: ILLEGAL, Literal: string(l.ch), Pos: pos}
	}

	l.readChar()
	return tok
}

func (l *Lexer) readIdentifier() string {
	start := l.pos
	for isLetter(l.ch) || isDigit(l.ch) {
		l.readChar()
	}
	return l.input[start:l.pos]
}

func (l *Lexer) readNumber() string {
	start := l.pos
	for isDigit(l.ch) {
		l.readChar()
	}
	return l.input[start:l.pos]
}

func (l *Lexer) readString() string {
	l.readChar()
	start := l.pos

	for l.ch != '\'' && l.ch != 0 {
		l.readChar()
	}

	lit := l.input[start:l.pos]
	l.readChar()
	return lit
}

func isLetter(ch byte) bool {
	return ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || ch == '_'
}

func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}
