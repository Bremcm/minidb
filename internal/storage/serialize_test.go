package storage

import (
	"testing"
)

// Главный тест: значение туда-обратно должно вернуться идентичным.
func TestValueRoundTrip(t *testing.T) {
	cases := []Value{
		NewInt(0),
		NewInt(1),
		NewInt(-1),
		NewInt(42),
		NewInt(9223372036854775807),  // max int64
		NewInt(-9223372036854775808), // min int64
		NewText(""),
		NewText("bob"),
		NewText("привет"), // UTF-8
		NewText("with spaces and, commas"),
		NewNull(),
	}

	for _, want := range cases {
		buf := AppendValue(nil, want)

		got, n, err := ReadValue(buf)
		if err != nil {
			t.Fatalf("ReadValue(%v): %v", want, err)
		}
		if n != len(buf) {
			t.Errorf("%v: прочитано %d байт из %d", want, n, len(buf))
		}
		if got != want {
			t.Errorf("round-trip разошёлся: записали %v, прочитали %v", want, got)
		}
	}
}

// Проверяем, что INT реально little-endian и занимает ровно 9 байт.
func TestIntEncoding(t *testing.T) {
	buf := AppendValue(nil, NewInt(1))

	if len(buf) != 9 {
		t.Fatalf("INT должен занимать 9 байт, занял %d", len(buf))
	}
	if buf[0] != byte(TypeInt) {
		t.Errorf("первый байт должен быть тип INT")
	}
	// Число 1 в little-endian: младший байт первым.
	if buf[1] != 1 {
		t.Errorf("байт данных: ожидал 1, получил %d", buf[1])
	}
	for i := 2; i < 9; i++ {
		if buf[i] != 0 {
			t.Errorf("байт %d должен быть 0, получил %d", i, buf[i])
		}
	}
}

// Проверяем формат TEXT: тип, длина, данные.
func TestTextEncoding(t *testing.T) {
	buf := AppendValue(nil, NewText("bob"))

	// 1 (тип) + 4 (длина) + 3 (строка) = 8
	if len(buf) != 8 {
		t.Fatalf("TEXT 'bob' должен занять 8 байт, занял %d", len(buf))
	}
	if buf[0] != byte(TypeText) {
		t.Errorf("первый байт должен быть тип TEXT")
	}
	// Длина 3 в little-endian: 03 00 00 00
	if buf[1] != 3 || buf[2] != 0 || buf[3] != 0 || buf[4] != 0 {
		t.Errorf("байты длины неверны: % x", buf[1:5])
	}
	if string(buf[5:]) != "bob" {
		t.Errorf("данные строки неверны: %q", string(buf[5:]))
	}
}

// Строка целиком: туда-обратно.
func TestRowRoundTrip(t *testing.T) {
	cases := []Row{
		{NewInt(1), NewText("bob"), NewInt(30)},
		{NewText("alice")},
		{NewInt(-5), NewNull(), NewText("")},
		{}, // пустая строка — тоже валидный случай
	}

	for _, want := range cases {
		buf := SerializeRow(want)

		got, err := DeserializeRow(buf)
		if err != nil {
			t.Fatalf("DeserializeRow(%v): %v", want, err)
		}

		if len(got) != len(want) {
			t.Fatalf("длина разошлась: записали %d значений, прочитали %d",
				len(want), len(got))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("значение %d: записали %v, прочитали %v",
					i, want[i], got[i])
			}
		}
	}
}

// Несколько значений подряд читаются с правильными границами.
func TestSequentialValues(t *testing.T) {
	var buf []byte
	buf = AppendValue(buf, NewInt(100))
	buf = AppendValue(buf, NewText("x"))
	buf = AppendValue(buf, NewInt(200))

	pos := 0

	v1, n1, _ := ReadValue(buf[pos:])
	pos += n1
	if v1 != NewInt(100) {
		t.Errorf("первое значение: %v", v1)
	}

	v2, n2, _ := ReadValue(buf[pos:])
	pos += n2
	if v2 != NewText("x") {
		t.Errorf("второе значение: %v", v2)
	}

	v3, _, _ := ReadValue(buf[pos:])
	if v3 != NewInt(200) {
		t.Errorf("третье значение: %v", v3)
	}
}

// Битые данные должны давать ошибку, а не панику.
func TestCorruptData(t *testing.T) {
	cases := [][]byte{
		{},                           // пусто
		{byte(TypeInt)},              // тип INT, но нет числа
		{byte(TypeInt), 1, 2},        // тип INT, число обрезано
		{byte(TypeText), 5, 0, 0, 0}, // TEXT длиной 5, но данных нет
		{99},                         // несуществующий тип
	}

	for i, buf := range cases {
		_, _, err := ReadValue(buf)
		if err == nil {
			t.Errorf("случай %d: ожидал ошибку на битых данных % x", i, buf)
		}
	}
}
