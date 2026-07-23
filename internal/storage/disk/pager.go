package disk

import "fmt"

type Logger interface {
	Begin() (uint64, error)
	WritePage(txID uint64, pageID PageID, page *Page) error
	Commit(txID uint64) error
	Truncate() error
}

type frame struct {
	page  Page
	id    PageID
	dirty bool
}

type Pager struct {
	dm     *DiskManager
	frames map[PageID]*frame
	log    Logger
}

func NewPager(dm *DiskManager) *Pager {
	return &Pager{
		dm:     dm,
		frames: make(map[PageID]*frame),
	}
}

func (p *Pager) SetLogger(l Logger) {
	p.log = l
}

func (p *Pager) FetchPage(id PageID) (*Page, error) {
	if f, ok := p.frames[id]; ok {
		return &f.page, nil
	}

	f := &frame{id: id}
	if err := p.dm.ReadPage(id, &f.page); err != nil {
		return nil, err
	}

	p.frames[id] = f
	return &f.page, nil
}

func (p *Pager) MarkDirty(id PageID) {
	if f, ok := p.frames[id]; ok {
		f.dirty = true
	}
}

func (p *Pager) AllocatePage() (PageID, *Page, error) {
	id, err := p.dm.AllocatePage()
	if err != nil {
		return 0, nil, err
	}

	f := &frame{id: id, dirty: true}
	InitPage(&f.page)

	p.frames[id] = f
	return id, &f.page, nil
}

func (p *Pager) FlushPage(id PageID) error {
	f, ok := p.frames[id]
	if !ok || !f.dirty {
		return nil
	}

	if err := p.dm.WritePage(id, &f.page); err != nil {
		return fmt.Errorf("сброс страницы %d: %w", id, err)
	}

	f.dirty = false
	return nil
}

func (p *Pager) FlushAll() error {
	dirty := make([]PageID, 0, len(p.frames))
	for id, f := range p.frames {
		if f.dirty {
			dirty = append(dirty, id)
		}
	}

	if len(dirty) == 0 {
		return nil
	}

	if p.log != nil {
		txID, err := p.log.Begin()
		if err != nil {
			return fmt.Errorf("начало транзакции журнала: %w", err)
		}

		for _, id := range dirty {
			f := p.frames[id]
			if err := p.log.WritePage(txID, id, &f.page); err != nil {
				return fmt.Errorf("запись страницы %d в журнал: %w", id, err)
			}
		}

		if err := p.log.Commit(txID); err != nil {
			return fmt.Errorf("коммит журнала: %w", err)
		}
	}

	for _, id := range dirty {
		f := p.frames[id]
		if err := p.dm.WritePage(id, &f.page); err != nil {
			return fmt.Errorf("запись страницы %d: %w", id, err)
		}
		f.dirty = false
	}

	if err := p.dm.Sync(); err != nil {
		return fmt.Errorf("sync файла данных: %w", err)
	}

	if p.log != nil {
		if err := p.log.Truncate(); err != nil {
			return fmt.Errorf("очистка журнала: %w", err)
		}
	}

	return nil
}

func (p *Pager) Close() error {
	if err := p.FlushAll(); err != nil {
		return err
	}
	return p.dm.Close()
}

func (p *Pager) NumPages() PageID {
	return p.dm.NumPages()
}
