package disk

import "fmt"

type frame struct {
	page  Page
	id    PageID
	dirty bool
}

type Pager struct {
	dm     *DiskManager
	frames map[PageID]*frame
}

func NewPager(dm *DiskManager) *Pager {
	return &Pager{
		dm:     dm,
		frames: make(map[PageID]*frame),
	}
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
	if !ok {
		return nil
	}
	if !f.dirty {
		return nil
	}

	if err := p.dm.WritePage(id, &f.page); err != nil {
		return fmt.Errorf("сброс страницы %d: %w", id, err)
	}

	f.dirty = false
	return nil
}

func (p *Pager) FlushAll() error {
	for id := range p.frames {
		if err := p.FlushPage(id); err != nil {
			return err
		}
	}
	return p.dm.Sync()
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
