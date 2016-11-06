package storage

import (
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/local/chunk"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/tecbot/gorocksdb"
	"golang.org/x/net/context"
)

const timeBucketFormat = "2006-01-02T15"

type RocksDB struct {
	db    *gorocksdb.DB
	index *gorocksdb.DB
}

func (r *RocksDB) Append(s *model.Sample) error {
	// FIXME: see (s *MemorySeriesStorage) Append(sample *model.Sample), delete empty metrics
	// makes sure we add a test for label order, they are sorted
	key = time.Now().Format(timeBucketFormat) + s.Metric.String()

	// FIXME: compare timestamp to last seen for this series, drop it if timestamps are equal

	chunk := chunk.NewForEncoding(chunk.Varbit)

	wo := gorocksdb.NewDefaultWriteOptions()
	defer wo.Destroy()
	err := r.db.Put(wo, []byte(key), []byte("bar"))
	return err
}

func (r *RocksDB) Start() error {
	// https://github.com/tecbot/gorocksdb/blob/master/doc.go
	filter := gorocksdb.NewBloomFilter(10)
	bbto := gorocksdb.NewDefaultBlockBasedTableOptions()
	bbto.SetBlockCache(gorocksdb.NewLRUCache(3 << 30))
	bbto.SetFilterPolicy(filter)
	opts := gorocksdb.NewDefaultOptions()
	opts.SetBlockBasedTableFactory(bbto)
	opts.SetCreateIfMissing(true)

	var err error
	r.db, err = gorocksdb.OpenDb(opts, "/tmp/athens-timeseries.db")
	if err != nil {
		return err
	}

	bbto = gorocksdb.NewDefaultBlockBasedTableOptions()
	opts = gorocksdb.NewDefaultOptions()
	opts.SetBlockBasedTableFactory(bbto)
	opts.SetCreateIfMissing(true)
	r.index, err = gorocksdb.OpenDb(opts, "/tmp/athens-index.db")

	return err
}

func (r *RocksDB) Stop() error {
	r.db.Close()
	r.index.Close()
	return nil
}

func (r *RocksDB) DropMetricsForLabelMatchers(context.Context, ...*metric.LabelMatcher) (int, error) {
	return 0, nil
}

// FIXME: remove this if we can use own storage interface
func (r *RocksDB) NeedsThrottling() bool {
	return false
}

// FIXME: remove this if we can use own storage interface
func (r *RocksDB) WaitForIndexing() {}

func (r *RocksDB) Querier() (local.Querier, error) {
	return &rocksDBQuerier{}, nil
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

func (r *RocksDBIterator) ValueAtOrBeforeTime(model.Time) model.SamplePair {
	return model.SamplePair{
		Timestamp: model.Now(),
		Value:     0.1111,
	}
}

func (r *RocksDBIterator) RangeValues(metric.Interval) []model.SamplePair {
	return []model.SamplePair{
		model.SamplePair{
			Timestamp: model.Now(),
			Value:     0.1111,
		},
	}
}

func (r *RocksDBIterator) Metric() metric.Metric {
	return r.metric
}

func (r *RocksDBIterator) Close() {
}
