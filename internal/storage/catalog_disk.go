package storage

import (
	"encoding/binary"
	"fmt"

	"github.com/Bremcm/minidb/internal/storage/disk"
)

func serializeCatalog(tables map[string]*Table) []byte {
	buf := make([]byte, 0, 512)

	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(tables)))

	for name, t := range tables {
		buf = binary.LittleEndian.AppendUint32(buf, uint32(len(name)))
		buf = append(buf, name...)

		buf = binary.LittleEndian.AppendUint32(buf, uint32(len(t.Columns)))
		for _, c := range t.Columns {
			buf = binary.LittleEndian.AppendUint32(buf, uint32(len(c.Name)))
			buf = append(buf, c.Name...)
			buf = append(buf, byte(c.Type))
		}

		buf = binary.LittleEndian.AppendUint32(buf, uint32(len(t.pages)))
		for _, pid := range t.pages {
			buf = binary.LittleEndian.AppendUint32(buf, uint32(pid))
		}
	}

	return buf
}

func deserializeCatalog(buf []byte) (map[string]*Table, error) {
	if len(buf) < 4 {
		return nil, fmt.Errorf("каталог: слишком короткий буфер")
	}

	pos := 0

	readU32 := func() (uint32, error) {
		if pos+4 > len(buf) {
			return 0, fmt.Errorf("каталог: обрыв на позиции %d", pos)
		}
		v := binary.LittleEndian.Uint32(buf[pos : pos+4])
		pos += 4
		return v, nil
	}

	readStr := func() (string, error) {
		n, err := readU32()
		if err != nil {
			return "", err
		}
		if pos+int(n) > len(buf) {
			return "", fmt.Errorf("каталог: строка длиной %d не влезает", n)
		}
		s := string(buf[pos : pos+int(n)])
		pos += int(n)
		return s, nil
	}

	numTables, err := readU32()
	if err != nil {
		return nil, err
	}

	tables := make(map[string]*Table, numTables)

	for i := uint32(0); i < numTables; i++ {
		name, err := readStr()
		if err != nil {
			return nil, err
		}

		numCols, err := readU32()
		if err != nil {
			return nil, err
		}

		cols := make([]Column, 0, numCols)
		for j := uint32(0); j < numCols; j++ {
			colName, err := readStr()
			if err != nil {
				return nil, err
			}
			if pos >= len(buf) {
				return nil, fmt.Errorf("каталог: обрыв на типе колонки")
			}
			colType := ValueType(buf[pos])
			pos++

			cols = append(cols, Column{Name: colName, Type: colType})
		}

		numPages, err := readU32()
		if err != nil {
			return nil, err
		}

		pages := make([]disk.PageID, 0, numPages)
		for j := uint32(0); j < numPages; j++ {
			pid, err := readU32()
			if err != nil {
				return nil, err
			}
			pages = append(pages, disk.PageID(pid))
		}

		tables[name] = &Table{
			Name:    name,
			Columns: cols,
			pages:   pages,
		}
	}

	return tables, nil
}
