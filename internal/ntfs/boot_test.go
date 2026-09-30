package ntfs

import (
	"encoding/binary"
	"errors"
	"testing"
)

func TestParseBootSector(t *testing.T) {
	t.Parallel()

	t.Run("decodes geometry and mft location", func(t *testing.T) {
		t.Parallel()

		sector := bootSectorFixture(t, func(b []byte) {
			binary.LittleEndian.PutUint16(b[11:13], 4096)
			b[13] = 8
			b[0x40] = 0xF6 // -10: a 1024-byte mft record
			binary.LittleEndian.PutUint64(b[0x30:0x38], 4)
			binary.LittleEndian.PutUint64(b[0x38:0x40], 8191)
		})

		got, err := ParseBootSector(sector)
		if err != nil {
			t.Fatalf("ParseBootSector() error = %v", err)
		}

		if got.ClusterSize() != 32768 {
			t.Errorf("ClusterSize() = %d, want 32768", got.ClusterSize())
		}

		if got.MFTRecordSize() != RecordSize {
			t.Errorf("MFTRecordSize() = %d, want %d", got.MFTRecordSize(), RecordSize)
		}

		if got.MFTCluster != 4 {
			t.Errorf("MFTCluster = %d, want 4", got.MFTCluster)
		}

		if got.MFTMirrorCluster != 8191 {
			t.Errorf("MFTMirrorCluster = %d, want 8191", got.MFTMirrorCluster)
		}
	})

	t.Run("derives a record size from a positive run length", func(t *testing.T) {
		t.Parallel()

		sector := bootSectorFixture(t, func(b []byte) {
			b[0x40] = 1 // one cluster per record
		})

		got, err := ParseBootSector(sector)
		if err != nil {
			t.Fatalf("ParseBootSector() error = %v", err)
		}

		if got.MFTRecordSize() != 4096 {
			t.Errorf("MFTRecordSize() = %d, want 4096", got.MFTRecordSize())
		}
	})

	t.Run("rejects a foreign signature", func(t *testing.T) {
		t.Parallel()

		sector := bootSectorFixture(t, func(b []byte) {
			copy(b[3:11], "MSWIN4.1")
		})

		if _, err := ParseBootSector(sector); !errors.Is(err, ErrNotNTFS) {
			t.Errorf("ParseBootSector() error = %v, want ErrNotNTFS", err)
		}
	})

	t.Run("rejects a truncated sector", func(t *testing.T) {
		t.Parallel()

		if _, err := ParseBootSector(make([]byte, 8)); err == nil {
			t.Error("ParseBootSector() error = nil, want a truncation error")
		}
	})

	t.Run("rejects a zero cluster geometry", func(t *testing.T) {
		t.Parallel()

		sector := bootSectorFixture(t, func(b []byte) {
			binary.LittleEndian.PutUint16(b[11:13], 0)
		})

		if _, err := ParseBootSector(sector); err == nil {
			t.Error("ParseBootSector() error = nil, want a geometry error")
		}
	})
}

func TestParseRunList(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		encoded     []byte
		wantRuns    []Run
		wantSize    int64
		wantErr     bool
		wantErrIsIs error
	}{
		{
			name:     "single run",
			encoded:  []byte{0x21, 0x04, 0x10, 0x00},
			wantRuns: []Run{{FirstCluster: 16, Length: 4}},
			wantSize: 4,
		},
		{
			name:     "negative delta walks backwards",
			encoded:  []byte{0x11, 0x08, 0x04, 0x11, 0x02, 0xFC},
			wantRuns: []Run{{FirstCluster: 4, Length: 8}, {FirstCluster: 0, Length: 2}},
			wantSize: 10,
		},
		{
			name:    "empty runlist",
			encoded: []byte{0x00},
			wantErr: true,
		},
		{
			name:    "zero field width",
			encoded: []byte{0x01, 0x04},
			wantErr: true,
		},
		{
			name:    "truncated entry",
			encoded: []byte{0x21, 0x04},
			wantErr: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runs, size, err := ParseRunList(tt.encoded, 0)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ParseRunList() error = nil, want an error")
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseRunList() error = %v", err)
			}

			if len(runs) != len(tt.wantRuns) {
				t.Fatalf("runs = %+v, want %+v", runs, tt.wantRuns)
			}

			for i, want := range tt.wantRuns {
				if runs[i] != want {
					t.Errorf("runs[%d] = %+v, want %+v", i, runs[i], want)
				}
			}

			if size != tt.wantSize {
				t.Errorf("size = %d, want %d", size, tt.wantSize)
			}
		})
	}
}

func TestParseRunListHonoursBaseCluster(t *testing.T) {
	t.Parallel()

	runs, _, err := ParseRunList([]byte{0x11, 0x02, 0x00}, 1024)
	if err != nil {
		t.Fatalf("ParseRunList() error = %v", err)
	}

	if runs[0].FirstCluster != 1024 {
		t.Errorf("runs[0].FirstCluster = %d, want 1024", runs[0].FirstCluster)
	}
}

func TestClusterCount(t *testing.T) {
	t.Parallel()

	runs := []Run{{Length: 4}, {Length: 6}, {Length: 2}}

	if got := ClusterCount(runs); got != 12 {
		t.Errorf("ClusterCount() = %d, want 12", got)
	}
}

func TestClusterAt(t *testing.T) {
	t.Parallel()

	runs := []Run{
		{FirstCluster: 100, Length: 2},
		{FirstCluster: 200, Length: 3},
	}

	cases := []struct {
		name      string
		index     int64
		want      uint64
		wantFound bool
	}{
		{"start of the first run", 0, 100, true},
		{"end of the first run", 1, 101, true},
		{"start of the second run", 2, 200, true},
		{"middle of the second run", 3, 201, true},
		{"past the end", 5, 0, false},
		{"negative", -1, 0, false},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, found := ClusterAt(runs, tt.index)
			if found != tt.wantFound {
				t.Fatalf("ClusterAt(%d) found = %t, want %t", tt.index, found, tt.wantFound)
			}

			if got != tt.want {
				t.Errorf("ClusterAt(%d) = %d, want %d", tt.index, got, tt.want)
			}
		})
	}
}

// bootSectorFixture returns a minimal decodable NTFS boot sector after mutate
// has adjusted it.
func bootSectorFixture(t *testing.T, mutate func([]byte)) []byte {
	t.Helper()

	sector := make([]byte, SectorSize)
	copy(sector[3:11], OemSignature)
	binary.LittleEndian.PutUint16(sector[11:13], 512)
	sector[13] = 8
	sector[0x40] = 0xF6 // a 1024-byte mft record

	mutate(sector)

	return sector
}
