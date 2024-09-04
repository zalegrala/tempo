package backend

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/grafana/tempo/tempodb/backend/meta"
	backend_v1 "github.com/grafana/tempo/tempodb/backend/v1"
)

type CompactedBlockMeta struct {
	CompactedTime time.Time `json:"compactedTime"`
	BlockMeta
}

func (b *CompactedBlockMeta) ToBackendV1Proto() (*backend_v1.CompactedBlockMeta, error) {
	bm, err := b.BlockMeta.ToBackendV1Proto()
	if err != nil {
		return nil, err
	}

	return &backend_v1.CompactedBlockMeta{
		BlockMeta:     *bm,
		CompactedTime: b.CompactedTime,
	}, nil
}

func (b *CompactedBlockMeta) FromBackendV1Proto(pb *backend_v1.CompactedBlockMeta) error {
	err := b.BlockMeta.FromBackendV1Proto(&pb.BlockMeta)
	if err != nil {
		return err
	}

	b.CompactedTime = pb.CompactedTime

	return nil
}

const (
	DefaultReplicationFactor          = 0 // Replication factor for blocks from the ingester. This is the default value to indicate RF3.
	MetricsGeneratorReplicationFactor = 1
)

// The BlockMeta data that is stored for each individual block.
type BlockMeta struct {
	// A Version that indicates the block format. This includes specifics of how the indexes and data is stored.
	Version string `json:"format"`
	// BlockID is a unique identifier of the block.
	BlockID uuid.UUID `json:"blockID"`
	// A TenantID that defines the tenant to which this block belongs.
	TenantID string `json:"tenantID"`
	// StartTime roughly matches when the first obj was written to this block. It is used to determine block.
	// age for different purposes (caching, etc)
	StartTime time.Time `json:"startTime"`
	// EndTime roughly matches to the time the last obj was written to this block. Is currently mostly meaningless.
	EndTime time.Time `json:"endTime"`
	// TotalObjects counts the number of objects in this block.
	TotalObjects int `json:"totalObjects"`
	// The Size in bytes of the block.
	Size uint64 `json:"size"`
	// CompactionLevel defines the number of times this block has been compacted.
	CompactionLevel uint8 `json:"compactionLevel"`
	// Encoding and compression format (used only in v2)
	Encoding Encoding `json:"encoding"`
	// IndexPageSize holds the size of each index page in bytes (used only in v2)
	IndexPageSize uint32 `json:"indexPageSize"`
	// TotalRecords holds the total Records stored in the index file (used only in v2)
	TotalRecords uint32 `json:"totalRecords"`
	// DataEncoding is tracked by tempodb and indicates the way the bytes are encoded.
	DataEncoding string `json:"dataEncoding"`
	// BloomShardCount represents the number of bloom filter shards.
	BloomShardCount uint16 `json:"bloomShards"`
	// FooterSize contains the size of the footer in bytes (used by parquet)
	FooterSize uint32 `json:"footerSize"`
	// DedicatedColumns configuration for attributes (used by vParquet3)
	DedicatedColumns meta.DedicatedColumns `json:"dedicatedColumns,omitempty"`
	// ReplicationFactor is the number of times the data written in this block has been replicated.
	// It's left unset if replication factor is 3. Default is 0 (RF3).
	ReplicationFactor uint8 `json:"replicationFactor,omitempty"`
}

func NewBlockMeta(tenantID string, blockID uuid.UUID, version string, encoding Encoding, dataEncoding string) *BlockMeta {
	return NewBlockMetaWithDedicatedColumns(tenantID, blockID, version, encoding, dataEncoding, nil)
}

func NewBlockMetaWithDedicatedColumns(tenantID string, blockID uuid.UUID, version string, encoding Encoding, dataEncoding string, dc meta.DedicatedColumns) *BlockMeta {
	b := &BlockMeta{
		Version:          version,
		BlockID:          blockID,
		TenantID:         tenantID,
		Encoding:         encoding,
		DataEncoding:     dataEncoding,
		DedicatedColumns: dc,
	}

	return b
}

// ObjectAdded updates the block meta appropriately based on information about an added record
// start/end are unix epoch seconds, when 0 the start and the end are not applied.
func (b *BlockMeta) ObjectAdded(start, end uint32) {
	if start > 0 {
		startTime := time.Unix(int64(start), 0)
		if b.StartTime.IsZero() || startTime.Before(b.StartTime) {
			b.StartTime = startTime
		}
	}

	if end > 0 {
		endTime := time.Unix(int64(end), 0)
		if b.EndTime.IsZero() || endTime.After(b.EndTime) {
			b.EndTime = endTime
		}
	}

	b.TotalObjects++
}

func (b *BlockMeta) DedicatedColumnsHash() uint64 {
	return b.DedicatedColumns.Hash()
}

func (b *BlockMeta) ToBackendV1Proto() (*backend_v1.BlockMeta, error) {
	blockID, err := b.BlockID.MarshalText()
	if err != nil {
		return nil, err
	}

	m := &backend_v1.BlockMeta{
		Version:           b.Version,
		BlockId:           blockID,
		TenantId:          b.TenantID,
		StartTime:         b.StartTime,
		EndTime:           b.EndTime,
		TotalObjects:      int32(b.TotalObjects),
		Size_:             b.Size,
		CompactionLevel:   uint32(b.CompactionLevel),
		Encoding:          uint32(b.Encoding),
		IndexPageSize:     b.IndexPageSize,
		TotalRecords:      b.TotalRecords,
		DataEncoding:      b.DataEncoding,
		BloomShardCount:   uint32(b.BloomShardCount),
		FooterSize:        b.FooterSize,
		ReplicationFactor: uint32(b.ReplicationFactor),
	}

	if len(b.DedicatedColumns) > 0 {
		bb, err := json.Marshal(b.DedicatedColumns)
		if err != nil {
			return nil, err
		}

		m.DedicatedColumns = bb
	}

	return m, nil
}

func (b *BlockMeta) FromBackendV1Proto(pb *backend_v1.BlockMeta) error {
	blockID, err := uuid.ParseBytes(pb.BlockId)
	if err != nil {
		return err
	}

	b.Version = pb.Version
	b.BlockID = blockID
	b.TenantID = pb.TenantId
	b.StartTime = pb.StartTime
	b.EndTime = pb.EndTime
	b.TotalObjects = int(pb.TotalObjects)
	b.Size = pb.Size_
	b.CompactionLevel = uint8(pb.CompactionLevel)
	b.Encoding = Encoding(pb.Encoding)
	b.IndexPageSize = pb.IndexPageSize
	b.TotalRecords = pb.TotalRecords
	b.DataEncoding = pb.DataEncoding
	b.BloomShardCount = uint16(pb.BloomShardCount)
	b.FooterSize = pb.FooterSize
	b.ReplicationFactor = uint8(pb.ReplicationFactor)

	if len(pb.DedicatedColumns) > 0 {
		err := json.Unmarshal(pb.DedicatedColumns, &b.DedicatedColumns)
		if err != nil {
			return err
		}
	}

	return nil
}
