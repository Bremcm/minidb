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

func (bt *BTree) Insert(key int64, data []byte) error {
	splitKey, newChild, didSplit, err := bt.insertInto(bt.root, key, data)
	if err != nil {
		return err
	}

	if !didSplit {
		return nil
	}

	newRootID, newRoot, err := bt.pager.AllocatePage()
	if err != nil {
		return err
	}

	disk.InitInternal(newRoot)
	disk.SetKeyForTest(newRoot, 0, splitKey)
	disk.SetNumKeysForTest(newRoot, 1)
	disk.SetChildForTest(newRoot, 0, bt.root)
	disk.SetChildForTest(newRoot, 1, newChild)

	bt.pager.MarkDirty(newRootID)
	bt.root = newRootID

	return nil
}

func (bt *BTree) insertInto(
	nodeID disk.PageID,
	key int64,
	data []byte,
) (splitKey int64, newChild disk.PageID, didSplit bool, err error) {

	page, err := bt.pager.FetchPage(nodeID)
	if err != nil {
		return 0, 0, false, err
	}

	if disk.IsLeaf(page) {
		return bt.insertIntoLeaf(nodeID, key, data)
	}

	return bt.insertIntoInternal(nodeID, key, data)
}

func (bt *BTree) insertIntoLeaf(
	leafID disk.PageID,
	key int64,
	data []byte,
) (int64, disk.PageID, bool, error) {

	page, err := bt.pager.FetchPage(leafID)
	if err != nil {
		return 0, 0, false, err
	}

	if err := disk.LeafInsert(page, key, data); err == nil {
		bt.pager.MarkDirty(leafID)
		return 0, 0, false, nil
	}

	rightID, rightPage, err := bt.pager.AllocatePage()
	if err != nil {
		return 0, 0, false, err
	}

	page, err = bt.pager.FetchPage(leafID)
	if err != nil {
		return 0, 0, false, err
	}

	sep := disk.SplitLeaf(page, rightPage)

	disk.SetNextLeaf(page, rightID)

	bt.pager.MarkDirty(leafID)
	bt.pager.MarkDirty(rightID)

	target := page
	targetID := leafID
	if key >= sep {
		target = rightPage
		targetID = rightID
	}

	if err := disk.LeafInsert(target, key, data); err != nil {
		return 0, 0, false, fmt.Errorf(
			"после разделения ключ %d всё равно не влез: %w", key, err)
	}
	bt.pager.MarkDirty(targetID)

	return sep, rightID, true, nil
}

func (bt *BTree) insertIntoInternal(
	nodeID disk.PageID,
	key int64,
	data []byte,
) (int64, disk.PageID, bool, error) {

	page, err := bt.pager.FetchPage(nodeID)
	if err != nil {
		return 0, 0, false, err
	}

	childIdx := disk.FindChild(page, key)
	childID := disk.ChildAt(page, childIdx)

	childSep, childNew, childSplit, err := bt.insertInto(childID, key, data)
	if err != nil {
		return 0, 0, false, err
	}

	if !childSplit {
		return 0, 0, false, nil
	}

	page, err = bt.pager.FetchPage(nodeID)
	if err != nil {
		return 0, 0, false, err
	}

	if err := disk.InternalInsert(page, childSep, childNew); err == nil {
		bt.pager.MarkDirty(nodeID)
		return 0, 0, false, nil
	}

	rightID, rightPage, err := bt.pager.AllocatePage()
	if err != nil {
		return 0, 0, false, err
	}

	page, err = bt.pager.FetchPage(nodeID)
	if err != nil {
		return 0, 0, false, err
	}

	upKey := disk.SplitInternal(page, rightPage)

	bt.pager.MarkDirty(nodeID)
	bt.pager.MarkDirty(rightID)

	if childSep < upKey {
		if err := disk.InternalInsert(page, childSep, childNew); err != nil {
			return 0, 0, false, err
		}
		bt.pager.MarkDirty(nodeID)
	} else {
		if err := disk.InternalInsert(rightPage, childSep, childNew); err != nil {
			return 0, 0, false, err
		}
		bt.pager.MarkDirty(rightID)
	}

	return upKey, rightID, true, nil
}
