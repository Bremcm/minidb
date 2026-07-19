package storage

import (
	"fmt"

	"github.com/Bremcm/minidb/internal/storage/disk"
)

type BTree struct {
	pager *disk.Pager
	root  disk.PageID
}

func NewBTree(pager *disk.Pager) (*BTree, error) {
	id, page, err := pager.AllocatePage()
	if err != nil {
		return nil, err
	}

	disk.InitLeaf(page)
	pager.MarkDirty(id)

	return &BTree{pager: pager, root: id}, nil
}

func OpenBTree(pager *disk.Pager, root disk.PageID) *BTree {
	return &BTree{pager: pager, root: root}
}

func (bt *BTree) Root() disk.PageID {
	return bt.root
}

func (bt *BTree) findLeaf(key int64) (disk.PageID, error) {
	id := bt.root

	for {
		page, err := bt.pager.FetchPage(id)
		if err != nil {
			return 0, err
		}

		if disk.IsLeaf(page) {
			return id, nil
		}

		childIdx := disk.FindChild(page, key)
		id = disk.ChildAt(page, childIdx)
	}
}

func (bt *BTree) Search(key int64) ([]byte, bool, error) {
	leafID, err := bt.findLeaf(key)
	if err != nil {
		return nil, false, err
	}

	page, err := bt.pager.FetchPage(leafID)
	if err != nil {
		return nil, false, err
	}

	idx, found := disk.SearchKey(page, key)
	if !found {
		return nil, false, nil
	}

	data, err := disk.LeafValueAt(page, idx)
	if err != nil {
		return nil, false, err
	}

	return data, true, nil
}

func (bt *BTree) Scan(startKey int64, fn func(key int64, data []byte) error) error {
	leafID, err := bt.findLeaf(startKey)
	if err != nil {
		return err
	}

	page, err := bt.pager.FetchPage(leafID)
	if err != nil {
		return err
	}
	startIdx, _ := disk.SearchKey(page, startKey)

	for leafID != disk.InvalidPageID {
		page, err := bt.pager.FetchPage(leafID)
		if err != nil {
			return err
		}

		n := disk.NumKeys(page)
		for i := startIdx; i < n; i++ {
			key := disk.KeyAt(page, i)

			data, err := disk.LeafValueAt(page, i)
			if err != nil {
				return fmt.Errorf("лист %d индекс %d: %w", leafID, i, err)
			}

			if err := fn(key, data); err != nil {
				return err
			}
		}

		startIdx = 0
		leafID = disk.NextLeaf(page)
	}

	return nil
}

func (bt *BTree) ScanAll(fn func(key int64, data []byte) error) error {
	return bt.Scan(-9223372036854775808, fn)
}

// insertIntoLeaf — временный хелпер: вставка без разделения.
// В части 3 его заменит полноценный Insert со сплитом.
func (bt *BTree) insertIntoLeaf(key int64, data []byte) error {
	leafID, err := bt.findLeaf(key)
	if err != nil {
		return err
	}

	page, err := bt.pager.FetchPage(leafID)
	if err != nil {
		return err
	}

	if err := disk.LeafInsert(page, key, data); err != nil {
		return err
	}

	bt.pager.MarkDirty(leafID)
	return nil
}
