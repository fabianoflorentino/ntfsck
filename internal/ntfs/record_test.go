package ntfs

import (
	"encoding/binary"
	"errors"
	"testing"
)

func TestDecodeRecordHeader(t *testing.T) {
	t.Parallel()

	t.Run("decodes flags and sizes", func(t *testing.T) {
		t.Parallel()

		record := recordFixture(t, RecordFlags(FlagInUse|FlagDirectory))

		got, err := DecodeRecordHeader(record)
		if err != nil {
			t.Fatalf("DecodeRecordHeader() error = %v", err)
		}

		if got.Magic != RecordMagic {
			t.Errorf("Magic = %q, want %q", got.Magic, RecordMagic)
		}

		if !got.IsInUse() {
			t.Error("IsInUse() = false, want true")
		}

		if !got.IsDirectory() {
			t.Error("IsDirectory() = false, want true")
		}

		if got.SequenceNumber != 3 {
			t.Errorf("SequenceNumber = %d, want 3", got.SequenceNumber)
		}
	})

	t.Run("rejects a bad magic", func(t *testing.T) {
		t.Parallel()

		record := make([]byte, RecordSize)
		copy(record, "XXXX")

		if _, err := DecodeRecordHeader(record); !errors.Is(err, ErrBadRecordMagic) {
			t.Errorf("DecodeRecordHeader() error = %v, want ErrBadRecordMagic", err)
		}
	})

	t.Run("rejects a truncated record", func(t *testing.T) {
		t.Parallel()

		record := make([]byte, 16)
		copy(record, RecordMagic)

		if _, err := DecodeRecordHeader(record); err == nil {
			t.Error("DecodeRecordHeader() error = nil, want a truncation error")
		}
	})

	t.Run("rejects an attribute offset past the record", func(t *testing.T) {
		t.Parallel()

		record := recordFixture(t, FlagInUse)
		binary.LittleEndian.PutUint16(record[20:22], RecordSize+1)

		if _, err := DecodeRecordHeader(record); err == nil {
			t.Error("DecodeRecordHeader() error = nil, want an out-of-range error")
		}
	})

	t.Run("rejects bytes in use past the record", func(t *testing.T) {
		t.Parallel()

		record := recordFixture(t, FlagInUse)
		binary.LittleEndian.PutUint32(record[24:28], RecordSize*2)

		if _, err := DecodeRecordHeader(record); err == nil {
			t.Error("DecodeRecordHeader() error = nil, want an out-of-range error")
		}
	})
}

func TestSystemName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		index uint64
		want  string
	}{
		{MFT, "$mft"},
		{MFTMirror, "$mftmirr"},
		{LogFile, "$logfile"},
		{RootDir, "."},
		{Extend, "$extend"},
		{FixedSystemFiles, ""},
		{9999, ""},
	}

	for _, tt := range cases {
		if got := SystemName(tt.index); got != tt.want {
			t.Errorf("SystemName(%d) = %q, want %q", tt.index, got, tt.want)
		}
	}
}

func TestDataRuns(t *testing.T) {
	t.Parallel()

	t.Run("decodes a non-resident data attribute", func(t *testing.T) {
		t.Parallel()

		record := dataAttributeFixture(t, 8, 64)

		runs, size, err := DataRuns(record)
		if err != nil {
			t.Fatalf("DataRuns() error = %v", err)
		}

		if size != 8192 {
			t.Errorf("content size = %d, want 8192", size)
		}

		want := Run{FirstCluster: 64, Length: 8}
		if len(runs) != 1 || runs[0] != want {
			t.Errorf("runs = %+v, want [%+v]", runs, want)
		}
	})

	t.Run("rejects a resident data attribute", func(t *testing.T) {
		t.Parallel()

		record := recordFixture(t, FlagInUse)
		attr := record[0x38:]
		binary.LittleEndian.PutUint32(attr[0:4], uint32(TypeData))
		binary.LittleEndian.PutUint32(attr[4:8], 24)
		attr[8] = 0 // resident
		binary.LittleEndian.PutUint32(attr[16:20], 16)

		if _, _, err := DataRuns(record); err == nil {
			t.Error("DataRuns() error = nil, want a resident-attribute error")
		}
	})

	t.Run("reports a missing data attribute", func(t *testing.T) {
		t.Parallel()

		record := recordFixture(t, FlagInUse)

		if _, _, err := DataRuns(record); !errors.Is(err, ErrAttributeNotFound) {
			t.Errorf("DataRuns() error = %v, want ErrAttributeNotFound", err)
		}
	})
}

func TestAttributesStopsAtEndOfList(t *testing.T) {
	t.Parallel()

	record := dataAttributeFixture(t, 8, 64)
	// The end of an attribute list is marked by 0xFFFFFFFF in place of a length.
	binary.LittleEndian.PutUint32(record[0x38+0x48:], ^uint32(0))

	header, err := DecodeRecordHeader(record)
	if err != nil {
		t.Fatalf("DecodeRecordHeader() error = %v", err)
	}

	it := Attributes(record, header)

	attr, ok := it.Next()
	if !ok || attr.Type != TypeData {
		t.Fatalf("first attribute = %+v, ok = %t, want the data attribute", attr, ok)
	}

	if _, ok := it.Next(); ok {
		t.Error("Next() after the end marker = ok, want exhaustion")
	}
}

// recordFixture builds a record header with the given flags at offset 0x38.
func recordFixture(t *testing.T, flags RecordFlags) []byte {
	t.Helper()

	record := make([]byte, RecordSize)
	copy(record[0:4], RecordMagic)
	binary.LittleEndian.PutUint16(record[4:6], 0x30)
	binary.LittleEndian.PutUint16(record[6:8], 1)
	binary.LittleEndian.PutUint16(record[16:18], 3)    // sequence number
	binary.LittleEndian.PutUint16(record[18:20], 1)    // hard links
	binary.LittleEndian.PutUint16(record[20:22], 0x38) // attributes offset
	binary.LittleEndian.PutUint16(record[22:24], uint16(flags))
	binary.LittleEndian.PutUint32(record[24:28], uint32(RecordSize))

	return record
}

// dataAttributeFixture builds a record whose only attribute is a non-resident
// $data describing one run of length clusters at firstCluster.
func dataAttributeFixture(t *testing.T, length uint64, firstCluster uint8) []byte {
	t.Helper()

	record := recordFixture(t, FlagInUse)
	attr := record[0x38:]

	binary.LittleEndian.PutUint32(attr[0:4], uint32(TypeData))
	binary.LittleEndian.PutUint32(attr[4:8], 0x44)
	attr[8] = 1 // non-resident
	binary.LittleEndian.PutUint16(attr[0x20:0x22], 0x40)
	binary.LittleEndian.PutUint64(attr[0x38:0x40], length*1024)

	attr[0x40] = 0x21
	attr[0x41] = byte(length)
	attr[0x42] = firstCluster

	return record
}
