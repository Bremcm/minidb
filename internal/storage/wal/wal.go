package wal

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"

	"github.com/Bremcm/minidb/internal/storage/disk"
)

type RecordType byte

const (
	RecordBegin  RecordType = 1
	RecordPage   RecordType = 2
	RecordCommit RecordType = 3
)

const (
	typeSize     = 1
	txIDSize     = 8
	pageIDSize   = 4
	checksumSize = 4

	beginRecordSize  = typeSize + txIDSize + checksumSize
	commitRecordSize = typeSize + txIDSize + checksumSize
	pageRecordSize   = typeSize + txIDSize + pageIDSize + disk.PageSize + checksumSize
)

type WAL struct {
	file   *os.File
	path   string
	nextTx uint64
}

func Open(path string) (*WAL, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("не могу открыть журнал %q: %w", path, err)
	}

	return &WAL{
		file:   file,
		path:   path,
		nextTx: 1,
	}, nil
}

func checksum(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

func (w *WAL) Begin() (uint64, error) {
	txID := w.nextTx
	w.nextTx++

	buf := make([]byte, 0, beginRecordSize)
	buf = append(buf, byte(RecordBegin))
	buf = binary.LittleEndian.AppendUint64(buf, txID)
	buf = binary.LittleEndian.AppendUint32(buf, checksum(buf))

	if _, err := w.file.Write(buf); err != nil {
		return 0, fmt.Errorf("запись BEGIN: %w", err)
	}

	return txID, nil
}

func (w *WAL) Commit(txID uint64) error {
	buf := make([]byte, 0, commitRecordSize)
	buf = append(buf, byte(RecordCommit))
	buf = binary.LittleEndian.AppendUint64(buf, txID)
	buf = binary.LittleEndian.AppendUint32(buf, checksum(buf))

	if _, err := w.file.Write(buf); err != nil {
		return fmt.Errorf("запись COMMIT: %w", err)
	}

	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("sync журнала: %w", err)
	}

	return nil
}

func (w *WAL) WritePage(txID uint64, pageID disk.PageID, page *disk.Page) error {
	buf := make([]byte, 0, pageRecordSize)

	buf = append(buf, byte(RecordPage))
	buf = binary.LittleEndian.AppendUint64(buf, txID)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(pageID))
	buf = append(buf, page[:]...)
	buf = binary.LittleEndian.AppendUint32(buf, checksum(buf))

	if _, err := w.file.Write(buf); err != nil {
		return fmt.Errorf("запись страницы %d в журнал: %w", pageID, err)
	}

	return nil
}

type Record struct {
	Type   RecordType
	TxID   uint64
	PageID disk.PageID
	Page   disk.Page
}

func ReadAll(path string) ([]Record, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	var records []Record

	for {
		rec, err := readRecord(file)
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		records = append(records, rec)
	}

	return records, nil
}

func readRecord(file *os.File) (Record, error) {
	var typeBuf [1]byte
	if _, err := io.ReadFull(file, typeBuf[:]); err != nil {
		return Record{}, err
	}

	recType := RecordType(typeBuf[0])

	switch recType {
	case RecordBegin, RecordCommit:
		return readShortRecord(file, recType)
	case RecordPage:
		return readPageRecord(file)
	default:
		return Record{}, fmt.Errorf("неизвестный тип записи: %d", recType)
	}
}

func readShortRecord(file *os.File, recType RecordType) (Record, error) {
	var buf [txIDSize + checksumSize]byte
	if _, err := io.ReadFull(file, buf[:]); err != nil {
		return Record{}, fmt.Errorf("обрыв в записи типа %d: %w", recType, err)
	}

	txID := binary.LittleEndian.Uint64(buf[0:8])
	stored := binary.LittleEndian.Uint32(buf[8:12])

	var check []byte
	check = append(check, byte(recType))
	check = append(check, buf[0:8]...)

	if computed := checksum(check); computed != stored {
		return Record{}, fmt.Errorf(
			"битая контрольная сумма: в файле %d, посчитали %d", stored, computed)
	}

	return Record{Type: recType, TxID: txID}, nil
}

func readPageRecord(file *os.File) (Record, error) {
	buf := make([]byte, txIDSize+pageIDSize+disk.PageSize+checksumSize)
	if _, err := io.ReadFull(file, buf); err != nil {
		return Record{}, fmt.Errorf("обрыв в записи страницы: %w", err)
	}

	txID := binary.LittleEndian.Uint64(buf[0:8])
	pageID := disk.PageID(binary.LittleEndian.Uint32(buf[8:12]))

	dataStart := 12
	dataEnd := dataStart + disk.PageSize
	stored := binary.LittleEndian.Uint32(buf[dataEnd : dataEnd+4])

	check := make([]byte, 0, 1+dataEnd)
	check = append(check, byte(RecordPage))
	check = append(check, buf[0:dataEnd]...)

	if computed := checksum(check); computed != stored {
		return Record{}, fmt.Errorf(
			"битая контрольная сумма страницы %d", pageID)
	}

	var rec Record
	rec.Type = RecordPage
	rec.TxID = txID
	rec.PageID = pageID
	copy(rec.Page[:], buf[dataStart:dataEnd])

	return rec, nil
}

func (w *WAL) Truncate() error {
	if err := w.file.Truncate(0); err != nil {
		return fmt.Errorf("очистка журнала: %w", err)
	}
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("перемотка журнала: %w", err)
	}
	return w.file.Sync()
}

func (w *WAL) Close() error {
	return w.file.Close()
}

func (w *WAL) Path() string {
	return w.path
}

// Проверка, что *WAL удовлетворяет интерфейсу disk.Logger.
// Если сигнатуры разойдутся — не скомпилируется здесь,
// а не в далёком месте использования.
var _ disk.Logger = (*WAL)(nil)
