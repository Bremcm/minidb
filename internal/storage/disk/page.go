package disk

const PageSize = 4096

type PageID uint32

type Page [PageSize]byte
