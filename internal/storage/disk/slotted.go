package disk

import (
	"encoding/binary"
	"fmt"
)

const (
	headerSize = 8
	slotSize   = 8
)

func InitPage(p *Page) {
	setNumSlots(p, 0)
	setFreeEnd(p, PageSize)
}

func numSlots(p *Page) uint32 {
	return binary.LittleEndian.Uint32(p[0:4])
}

func setNumSlots(p *Page, n uint32) {
	binary.LittleEndian.PutUint32(p[0:4], n)
}

func freeEnd(p *Page) uint32 {
	return binary.LittleEndian.Uint32(p[4:8])
}

func setFreeEnd(p *Page, v uint32) {
	binary.LittleEndian.PutUint32(p[4:8], v)
}

func slotOffset(i uint32) uint32 {
	return headerSize + i*slotSize
}

func readSlot(p *Page, i uint32) (offset, length uint32) {
	base := slotOffset(i)
	offset = binary.LittleEndian.Uint32(p[base : base+4])
	length = binary.LittleEndian.Uint32(p[base+4 : base+8])
	return offset, length
}

func writeSlot(p *Page, i, offset, length uint32) {
	base := slotOffset(i)
	binary.LittleEndian.PutUint32(p[base:base+4], offset)
	binary.LittleEndian.PutUint32(p[base+4:base+8], length)
}

func FreeSpace(p *Page) uint32 {
	slotsEnd := slotOffset(numSlots(p))
	dataStart := freeEnd(p)

	if dataStart <= slotsEnd {
		return 0
	}
	return dataStart - slotsEnd
}

func InsertRow(p *Page, data []byte) (uint32, error) {
	dataLen := uint32(len(data))

	needed := dataLen + slotSize

	if FreeSpace(p) < needed {
		return 0, fmt.Errorf("нет места: нужно %d байт, свободно %d",
			needed, FreeSpace(p))
	}

	newFreeEnd := freeEnd(p) - dataLen
	copy(p[newFreeEnd:newFreeEnd+dataLen], data)

	slot := numSlots(p)
	writeSlot(p, slot, newFreeEnd, dataLen)

	setFreeEnd(p, newFreeEnd)
	setNumSlots(p, slot+1)

	return slot, nil
}

func ReadRow(p *Page, i uint32) ([]byte, error) {
	n := numSlots(p)
	if i >= n {
		return nil, fmt.Errorf("слот %d не существует (всего слотов %d)", i, n)
	}

	offset, length := readSlot(p, i)

	if offset+length > PageSize {
		return nil, fmt.Errorf(
			"слот %d повреждён: offset=%d length=%d выходит за страницу",
			i, offset, length)
	}

	out := make([]byte, length)
	copy(out, p[offset:offset+length])

	return out, nil
}

func NumRows(p *Page) uint32 {
	return numSlots(p)
}
