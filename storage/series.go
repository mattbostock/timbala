package storage

import (
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage/local/chunk"
)

type rocksDBSeries struct {
	metric model.Metric
	// Sorted by start time, overlapping chunk ranges are forbidden.
	chunkDescs []*chunk.Desc
	// The timestamp of the last sample in this series. Needed for fast
	// access for federation and to ensure timestamp monotonicity during
	// ingestion.
	lastTime model.Time
	// The last ingested sample value. Needed for fast access for
	// federation.
	lastSampleValue model.SampleValue
	// Whether lastSampleValue has been set already.
	lastSampleValueSet bool
	// Whether the current head chunk has already been finished.  If true,
	// the current head chunk must not be modified anymore.
	headChunkClosed bool
}

// add adds a sample pair to the series. It returns the number of newly
// completed chunks (which are now eligible for persistence).
//
// The caller must have locked the fingerprint of the series.
func (s *RocksDBSeries) add(v model.SamplePair) (int, error) {
	// FIXME: see (s *MemorySeriesStorage) Append(sample *model.Sample), delete empty metrics
	// makes sure we add a test for label order, they are sorted
	_ = time.Now().Format(timeBucketFormat) + s.Metric.String()

	// FIXME: compare timestamp to last seen for this series, drop it if timestamps are equal

	wo := gorocksdb.NewDefaultWriteOptions()
	defer wo.Destroy()
	err := s.db.Put(wo, []byte("foo"), []byte("bar"))
	return err
	return series, nil
}
