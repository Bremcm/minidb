package disk

import (
	"encoding/binary"
	"fmt"
)

type NodeType byte

const (
	NodeLeaf     NodeType = 1
	NodeInternal NodeType = 2
)

const (
	nodeHeaderSize = 16
	keySize        = 8
	childSize      = 4
	dataSlotSize   = 8
)

const InvalidPageID PageID = 0xFFFFFFFF

func nodeType(p *Page) NodeType {
	return NodeType(p[0])
}

func setNodeType(p *Page, t NodeType) {
	p[0] = byte(t)
}

func numKeys(p *Page) uint32 {
	return binary.LittleEndian.Uint32(p[4:8])
}

func setNumKeys(p *Page, n uint32) {
	binary.LittleEndian.PutUint32(p[4:8], n)
}

func nodeFreeEnd(p *Page) uint32 {
	return binary.LittleEndian.Uint32(p[8:12])
}

func setNodeFreeEnd(p *Page, v uint32) {
	binary.LittleEndian.PutUint32(p[8:12], v)
}

func nextLeaf(p *Page) PageID {
	return PageID(binary.LittleEndian.Uint32(p[12:16]))
}

func setNextLeaf(p *Page, id PageID) {
	binary.LittleEndian.PutUint32(p[12:16], uint32(id))
}

func InitLeaf(p *Page) {
	setNodeType(p, NodeLeaf)
	setNumKeys(p, 0)
	setNodeFreeEnd(p, PageSize)
	setNextLeaf(p, InvalidPageID)
}

func InitInternal(p *Page) {
	setNodeType(p, NodeInternal)
	setNumKeys(p, 0)
	setNodeFreeEnd(p, PageSize)
	setNextLeaf(p, InvalidPageID)
}

func IsLeaf(p *Page) bool {
	return nodeType(p) == NodeLeaf
}

func NumKeys(p *Page) uint32 {
	return numKeys(p)
}

func NextLeaf(p *Page) PageID {
	return nextLeaf(p)
}

func SetNextLeaf(p *Page, id PageID) {
	setNextLeaf(p, id)
}

func keyOffset(i uint32) uint32 {
	return nodeHeaderSize + i*keySize
}

func KeyAt(p *Page, i uint32) int64 {
	base := keyOffset(i)
	return int64(binary.LittleEndian.Uint64(p[base : base+8]))
}

func setKeyAt(p *Page, i uint32, key int64) {
	base := keyOffset(i)
	binary.LittleEndian.PutUint64(p[base:base+8], uint64(key))
}

func SearchKey(p *Page, key int64) (uint32, bool) {
	lo, hi := uint32(0), numKeys(p)

	for lo < hi {
		mid := lo + (hi-lo)/2
		k := KeyAt(p, mid)

		switch {
		case k == key:
			return mid, true
		case k < key:
			lo = mid + 1
		default:
			hi = mid
		}
	}

	return lo, false
}

const maxLeafKeys = 200

func leafSlotOffset(i uint32) uint32 {
	return nodeHeaderSize + maxLeafKeys*keySize + i*dataSlotSize
}

func readLeafSlot(p *Page, i uint32) (offset, length uint32) {
	base := leafSlotOffset(i)
	offset = binary.LittleEndian.Uint32(p[base : base+4])
	length = binary.LittleEndian.Uint32(p[base+4 : base+8])
	return
}

func writeLeafSlot(p *Page, i, offset, length uint32) {
	base := leafSlotOffset(i)
	binary.LittleEndian.PutUint32(p[base:base+4], offset)
	binary.LittleEndian.PutUint32(p[base+4:base+8], length)
}

func LeafValueAt(p *Page, i uint32) ([]byte, error) {
	if i >= numKeys(p) {
		return nil, fmt.Errorf("индекс %d вне диапазона (ключей %d)", i, numKeys(p))
	}

	offset, length := readLeafSlot(p, i)

	if offset+length > PageSize || offset < nodeHeaderSize {
		return nil, fmt.Errorf("повреждённый слот %d: offset=%d length=%d",
			i, offset, length)
	}

	out := make([]byte, length)
	copy(out, p[offset:offset+length])
	return out, nil
}

func LeafFreeSpace(p *Page) uint32 {
	used := leafSlotOffset(numKeys(p))
	dataStart := nodeFreeEnd(p)

	if dataStart <= used {
		return 0
	}
	return dataStart - used
}

func LeafInsert(p *Page, key int64, data []byte) error {
	if numKeys(p) >= maxLeafKeys {
		return fmt.Errorf("лист заполнен: %d ключей", numKeys(p))
	}

	pos, found := SearchKey(p, key)
	if found {
		return fmt.Errorf("ключ %d уже существует", key)
	}

	dataLen := uint32(len(data))
	needed := dataLen + dataSlotSize + keySize

	if LeafFreeSpace(p) < needed {
		return fmt.Errorf("нет места: нужно %d, свободно %d",
			needed, LeafFreeSpace(p))
	}

	n := numKeys(p)

	for i := n; i > pos; i-- {
		setKeyAt(p, i, KeyAt(p, i-1))

		off, ln := readLeafSlot(p, i-1)
		writeLeafSlot(p, i, off, ln)
	}

	newFreeEnd := nodeFreeEnd(p) - dataLen
	copy(p[newFreeEnd:newFreeEnd+dataLen], data)

	setKeyAt(p, pos, key)
	writeLeafSlot(p, pos, newFreeEnd, dataLen)

	setNodeFreeEnd(p, newFreeEnd)
	setNumKeys(p, n+1)

	return nil
}

func childOffset(i uint32) uint32 {
	return nodeHeaderSize + maxLeafKeys*keySize + i*childSize
}

func ChildAt(p *Page, i uint32) PageID {
	base := childOffset(i)
	return PageID(binary.LittleEndian.Uint32(p[base : base+4]))
}

func setChildAt(p *Page, i uint32, id PageID) {
	base := childOffset(i)
	binary.LittleEndian.PutUint32(p[base:base+4], uint32(id))
}

func FindChild(p *Page, key int64) uint32 {
	pos, found := SearchKey(p, key)
	if found {
		return pos + 1
	}
	return pos
}

// Экспортированные сеттеры для сборки узлов извне пакета.
// В части 3 они понадобятся при разделении узлов.

func SetKeyForTest(p *Page, i uint32, key int64) {
	setKeyAt(p, i, key)
}

func SetNumKeysForTest(p *Page, n uint32) {
	setNumKeys(p, n)
}

func SetChildForTest(p *Page, i uint32, id PageID) {
	setChildAt(p, i, id)
}

// ------------------------------ //

const maxInternalKeys = maxLeafKeys - 1

func InternalInsert(p *Page, key int64, child PageID) error {
	n := numKeys(p)
	if n >= maxInternalKeys {
		return fmt.Errorf("внутренний узел заполнен: %d ключей", n)
	}

	pos, _ := SearchKey(p, key)

	for i := n; i > pos; i-- {
		setKeyAt(p, i, KeyAt(p, i-1))
	}

	for i := n + 1; i > pos+1; i-- {
		setChildAt(p, i, ChildAt(p, i-1))
	}

	setKeyAt(p, pos, key)
	setChildAt(p, pos+1, child)
	setNumKeys(p, n+1)

	return nil
}

func SplitLeaf(left *Page, right *Page) int64 {
	n := numKeys(left)
	mid := n / 2

	oldNext := nextLeaf(left)

	InitLeaf(right)

	for i := mid; i < n; i++ {
		key := KeyAt(left, i)
		offset, length := readLeafSlot(left, i)
		data := left[offset : offset+length]
		if err := LeafInsert(right, key, data); err != nil {
			panic(fmt.Sprintf("SplitLeaf: правая половина не влезла: %v", err))
		}
	}

	var tmp Page
	InitLeaf(&tmp)
	for i := uint32(0); i < mid; i++ {
		key := KeyAt(left, i)
		offset, length := readLeafSlot(left, i)
		data := left[offset : offset+length]
		if err := LeafInsert(&tmp, key, data); err != nil {
			panic(fmt.Sprintf("SplitLeaf: левая половина не влезла: %v", err))
		}
	}
	*left = tmp

	setNextLeaf(right, oldNext)

	return KeyAt(right, 0)
}

func SplitInternal(left *Page, right *Page) int64 {
	n := numKeys(left)
	mid := n / 2

	upKey := KeyAt(left, mid)

	InitInternal(right)

	rightKeys := uint32(0)
	for i := mid + 1; i < n; i++ {
		setKeyAt(right, rightKeys, KeyAt(left, i))
		rightKeys++
	}

	for i := uint32(0); i <= rightKeys; i++ {
		setChildAt(right, i, ChildAt(left, mid+1+i))
	}

	setNumKeys(right, rightKeys)
	setNumKeys(left, mid)

	return upKey
}
