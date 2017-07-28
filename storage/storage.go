package storage

import (
	"fmt"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/tecbot/gorocksdb"
	"golang.org/x/net/context"
)

const timeBucketFormat = "2006-01-02T15"

var (
	// ErrOutOfOrderSample is returned if a sample has a timestamp before the latest
	// timestamp in the series it is appended to.
	ErrOutOfOrderSample = fmt.Errorf("sample timestamp out of order")
	// ErrDuplicateSampleForTimestamp is returned if a sample has the same
	// timestamp as the latest sample in the series it is appended to but a
	// different value. (Appending an identical sample is a no-op and does
	// not cause an error.)
	ErrDuplicateSampleForTimestamp = fmt.Errorf("sample with repeated timestamp but different value")
)

type RocksDB struct {
	db          *gorocksdb.DB
	index       *gorocksdb.DB
	storagePath string
}

func (s *RocksDB) Append(s *model.Sample) error {
	for ln, lv := range sample.Metric {
		if len(lv) == 0 {
			delete(sample.Metric, ln)
		}
	}

	/*rawFP := sample.Metric.FastFingerprint()
	s.fpLocker.Lock(rawFP)
	fp := s.mapper.mapFP(rawFP, sample.Metric)
	defer func() {
		s.fpLocker.Unlock(fp)
	}() // Func wrapper because fp might change below.

	if fp != rawFP {
		// Switch locks.
		s.fpLocker.Unlock(rawFP)
		s.fpLocker.Lock(fp)
	}*/

	series, err := s.getOrCreateSeries(sample.Metric)
	if err != nil {
		return err
	}

	// Comment this out for now - we'd need locking to track these errors reliably
	/*if sample.Timestamp == series.lastTime {
		// Don't report "no-op appends", i.e. where timestamp and sample
		// value are the same as for the last append, as they are a
		// common occurrence when using client-side timestamps
		// (e.g. Pushgateway or federation).
		// FIXME: Investigate what's required to remove this limitation.
		if sample.Timestamp == series.lastTime &&
			series.lastSampleValueSet &&
			sample.Value.Equal(series.lastSampleValue) {
			return nil
		}
		//s.discardedSamplesCount.WithLabelValues(duplicateSample).Inc()
		return ErrDuplicateSampleForTimestamp // Caused by the caller.
	}

	if sample.Timestamp < series.lastTime {
		s.discardedSamplesCount.WithLabelValues(outOfOrderTimestamp).Inc()
		return ErrOutOfOrderSample // Caused by the caller.
	}*/

	completedChunksCount, err := series.add(model.SamplePair{
		Value:     sample.Value,
		Timestamp: sample.Timestamp,
	})

	fmt.Println(completedChunksCount)

	if err != nil {
		panic("Not implemented")
		//s.quarantineSeries(fp, sample.Metric, err)
		return err
	}

	//s.ingestedSamplesCount.Inc()
	//s.incNumChunksToPersist(completedChunksCount)

	return nil
}

func (s *RocksDB) Start() error {
	s.storagePath = "/tmp"

	// https://github.com/tecbot/gorocksdb/blob/master/doc.go
	filter := gorocksdb.NewBloomFilter(10)
	bbto := gorocksdb.NewDefaultBlockBasedTableOptions()
	bbto.SetBlockCache(gorocksdb.NewLRUCache(3 << 30))
	bbto.SetFilterPolicy(filter)
	opts := gorocksdb.NewDefaultOptions()
	opts.SetBlockBasedTableFactory(bbto)
	opts.SetCreateIfMissing(true)

	var err error
	s.db, err = gorocksdb.OpenDb(opts, file.Join(s.storagePath, "athens-timeseries.db"))
	if err != nil {
		return err
	}

	bbto = gorocksdb.NewDefaultBlockBasedTableOptions()
	opts = gorocksdb.NewDefaultOptions()
	opts.SetBlockBasedTableFactory(bbto)
	opts.SetCreateIfMissing(true)
	s.index, err = gorocksdb.OpenDb(opts, file.Join(s.storagePath, "athens-index.db"))

	return err
}

func (s *RocksDB) Stop() error {
	s.db.Close()
	s.index.Close()
	return nil
}

func (s *RocksDB) DropMetricsForLabelMatchers(context.Context, ...*metric.LabelMatcher) (int, error) {
	return 0, nil
}

// FIXME: remove this if we can use own storage interface
func (s *RocksDB) NeedsThrottling() bool {
	return false
}

// FIXME: remove this if we can use own storage interface
func (s *RocksDB) WaitForIndexing() {}

func (s *RocksDB) Querier() (local.Querier, error) {
	return &rocksDBQuerier{}, nil
}

func (s *RocksDB) getOrCreateSeries(m model.Metric) (*memorySeries, error) {
	/*series, ok := s.fpToSeries.get(fp)
	if !ok {
		var cds []*chunk.Desc
		var modTime time.Time
		unarchived, err := s.persistence.unarchiveMetric(fp)
		if err != nil {
			log.Errorf("Error unarchiving fingerprint %v (metric %v): %v", fp, m, err)
			return nil, err
		}
		if unarchived {
			s.seriesOps.WithLabelValues(unarchive).Inc()
			// We have to load chunk.Descs anyway to do anything with
			// the series, so let's do it right now so that we don't
			// end up with a series without any chunk.Descs for a
			// while (which is confusing as it makes the series
			// appear as archived or purged).
			cds, err = s.loadChunkDescs(fp, 0)
			if err != nil {
				s.quarantineSeries(fp, m, err)
				return nil, err
			}
			modTime = s.persistence.seriesFileModTime(fp)
		} else {
			// This was a genuinely new series, so index the metric.
			s.persistence.indexMetric(fp, m)
			s.seriesOps.WithLabelValues(create).Inc()
		}
		series, err = newMemorySeries(m, cds, modTime)
		if err != nil {
			s.quarantineSeries(fp, m, err)
			return nil, err
		}
		s.fpToSeries.put(fp, series)
		s.numSeries.Inc()
	}*/

}

// rocksDBQuerier implements Querier
type rocksDBQuerier struct{}

func (q *rocksDBQuerier) Close() error {
	return nil
}

func (q *rocksDBQuerier) QueryRange(ctx context.Context, from, through model.Time, matchers ...*metric.LabelMatcher) ([]local.SeriesIterator, error) {
	return []local.SeriesIterator{&RocksDBIterator{metric: metric.Metric{}}}, nil
}

func (q *rocksDBQuerier) QueryInstant(ctx context.Context, ts model.Time, stalenessDelta time.Duration, matchers ...*metric.LabelMatcher) ([]local.SeriesIterator, error) {
	return []local.SeriesIterator{&RocksDBIterator{metric: metric.Metric{}}}, nil

}

func (q *rocksDBQuerier) MetricsForLabelMatchers(ctx context.Context, from, through model.Time, matcherSets ...metric.LabelMatchers) ([]metric.Metric, error) {
	return []metric.Metric{}, nil

}

func (q *rocksDBQuerier) LastSampleForLabelMatchers(ctx context.Context, cutoff model.Time, matcherSets ...metric.LabelMatchers) (model.Vector, error) {
	return model.Vector{}, nil
}

func (q *rocksDBQuerier) LabelValuesForLabelName(context.Context, model.LabelName) (model.LabelValues, error) {
	return model.LabelValues{}, nil
}

type RocksDBIterator struct {
	metric metric.Metric
}

func (s *RocksDBIterator) ValueAtOrBeforeTime(model.Time) model.SamplePair {
	return model.SamplePair{
		Timestamp: model.Now(),
		Value:     0.1111,
	}
}

func (s *RocksDBIterator) RangeValues(metric.Interval) []model.SamplePair {
	return []model.SamplePair{
		model.SamplePair{
			Timestamp: model.Now(),
			Value:     0.1111,
		},
	}
}

func (s *RocksDBIterator) Metric() metric.Metric {
	return s.metric
}

func (s *RocksDBIterator) Close() {
}
