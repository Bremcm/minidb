package disk

import (
	"fmt"
	"os"
)

type DiskManager struct {
	file     *os.File
	numPages PageID
}

func NewDiskManager(path string) (*DiskManager, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("не могу открыть файл БД %q: %w", path, err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("не могу получить размер файла: %w", err)
	}

	size := info.Size()
	if size%PageSize != 0 {
		file.Close()
		return nil, fmt.Errorf(
			"файл повреждён: размер %d не кратен размеру страницы %d",
			size, PageSize)
	}

	return &DiskManager{
		file:     file,
		numPages: PageID(size / PageSize),
	}, nil
}

func (dm *DiskManager) ReadPage(id PageID, page *Page) error {
	offset := int64(id) * PageSize

	n, err := dm.file.ReadAt(page[:], offset)
	if err != nil {
		return fmt.Errorf("чтение страницы %d: %w", id, err)
	}
	if n != PageSize {
		return fmt.Errorf("страница %d: прочитано %d байт вместо %d",
			id, n, PageSize)
	}

	return nil
}

func (dm *DiskManager) WritePage(id PageID, page *Page) error {
	offset := int64(id) * PageSize

	n, err := dm.file.WriteAt(page[:], offset)
	if err != nil {
		return fmt.Errorf("запись страницы %d: %w", id, err)
	}
	if n != PageSize {
		return fmt.Errorf("страница %d: записано %d байт вместо %d",
			id, n, PageSize)
	}

	return nil
}

func (dm *DiskManager) AllocatePage() (PageID, error) {
	id := dm.numPages

	var empty Page
	if err := dm.WritePage(id, &empty); err != nil {
		return 0, err
	}

	dm.numPages++
	return id, nil
}

func (dm *DiskManager) Sync() error {
	return dm.file.Sync()
}

func (dm *DiskManager) Close() error {
	return dm.file.Close()
}

func (dm *DiskManager) NumPages() PageID {
	return dm.numPages
}
