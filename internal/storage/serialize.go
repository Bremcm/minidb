package storage

import (
	"encoding/binary"
	"fmt"
)

func AppendValue(buf []byte, v Value) []byte {
	switch v.Type {
	case TypeNull:
		return append(buf, byte(TypeNull))

	case TypeInt:
		buf = append(buf, byte(TypeInt))
		buf = binary.LittleEndian.AppendUint64(buf, uint64(v.Int))
		return buf

	case TypeText:
		buf = append(buf, byte(TypeText))
		buf = binary.LittleEndian.AppendUint32(buf, uint32(len(v.Text)))
		buf = append(buf, v.Text...)
		return buf
	}

	panic(fmt.Sprintf("AppendValue: неизвестный тип %d", v.Type))
}

func ReadValue(buf []byte) (Value, int, error) {
	if len(buf) < 1 {
		return Value{}, 0, fmt.Errorf("нет данных для чтения типа")
	}

	typ := ValueType(buf[0])

	switch typ {
	case TypeNull:
		return NewNull(), 1, nil

	case TypeInt:
		if len(buf) < 9 {
			return Value{}, 0, fmt.Errorf(
				"нужно 9 байт для INT, есть %d", len(buf))
		}
		raw := binary.LittleEndian.Uint64(buf[1:9])
		return NewInt(int64(raw)), 9, nil

	case TypeText:
		if len(buf) < 5 {
			return Value{}, 0, fmt.Errorf(
				"нужно минимум 5 байт для TEXT, есть %d", len(buf))
		}
		strLen := int(binary.LittleEndian.Uint32(buf[1:5]))

		total := 5 + strLen
		if len(buf) < total {
			return Value{}, 0, fmt.Errorf(
				"TEXT длиной %d: нужно %d байт, есть %d",
				strLen, total, len(buf))
		}

		text := string(buf[5:total])
		return NewText(text), total, nil
	}

	return Value{}, 0, fmt.Errorf("неизвестный байт типа: %d", buf[0])
}

func SerializeRow(row Row) []byte {
	buf := make([]byte, 0, 64)

	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(row)))

	for _, v := range row {
		buf = AppendValue(buf, v)
	}

	return buf
}

func DeserializeRow(buf []byte) (Row, error) {
	if len(buf) < 4 {
		return nil, fmt.Errorf("нужно минимум 4 байта для заголовка строки")
	}

	count := int(binary.LittleEndian.Uint32(buf[0:4]))
	pos := 4

	row := make(Row, 0, count)

	for i := 0; i < count; i++ {
		v, n, err := ReadValue(buf[pos:])
		if err != nil {
			return nil, fmt.Errorf("значение %d: %w", i, err)
		}
		row = append(row, v)
		pos += n
	}

	return row, nil
}
