package read

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/mattbostock/timbala/internal/cluster"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/sirupsen/logrus"
)

const (
	HTTPHeaderInternalRead        = "X-Timbala-Internal-Read-Version"
	HTTPHeaderInternalReadVersion = "0.0.1"
	HTTPHeaderRemoteRead          = "X-Prometheus-Remote-Read-Version"
	HTTPHeaderRemoteReadVersion   = "0.1.0"
	Route                         = "/read"
)

type Reader interface {
	HandlerFunc(http.ResponseWriter, *http.Request)
}

type reader struct {
	clstr       cluster.Cluster
	fanoutStore storage.Storage
	localStore  storage.Storage
	log         *logrus.Logger
}

func New(c cluster.Cluster, l *logrus.Logger, s storage.Storage, fo storage.Storage) *reader {
	return &reader{
		clstr:       c,
		fanoutStore: fo,
		localStore:  s,
		log:         l,
	}
}

func (re *reader) HandlerFunc(w http.ResponseWriter, r *http.Request) {
	compressed, err := ioutil.ReadAll(r.Body)
	if err != nil {
		re.log.Debug(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		re.log.Debug(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req prompb.ReadRequest
	if err := proto.Unmarshal(reqBuf, &req); err != nil {
		re.log.Debug(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp := &prompb.ReadResponse{
		Results: make([]*prompb.QueryResult, len(req.Queries)),
	}

	internal := len(r.Header.Get(HTTPHeaderInternalRead)) > 0
	for i, query := range req.Queries {
		// FIXME paralellise queries
		matchers, err := fromLabelMatchers(query.Matchers)
		if err != nil {
			re.log.Debug(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var querier storage.Querier
		if internal {
			querier, err = re.localStore.Querier(r.Context(), query.StartTimestampMs, query.EndTimestampMs)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				re.log.Error(err)
				return
			}
		} else {
			querier, err = re.fanoutStore.Querier(r.Context(), query.StartTimestampMs, query.EndTimestampMs)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				re.log.Error(err)
				return
			}
		}
		defer querier.Close()

		sset := querier.Select(matchers...)
		resp.Results[i], err = ToQueryResult(sset)
		if err != nil {
			re.log.Warning(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	data, err := proto.Marshal(resp)
	if err != nil {
		re.log.Warning(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-protobuf")
	w.Header().Set("Content-Encoding", "snappy")

	compressed = snappy.Encode(nil, data)
	_, err = w.Write(compressed)
	if err != nil {
		re.log.Warning(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		re.log.Error(err)
		return
	}
}

// BEGIN FIXME: Use upstream versions of the following functions once they hit the dev-2.0 branch
// See: https://github.com/prometheus/prometheus/commit/639d5c6f98a6bf9790dbcc8d29d2ebcc14e40995

// FromQuery unpacks a Query proto.
func FromQuery(req *prompb.Query) (model.Time, model.Time, []*labels.Matcher, error) {
	matchers, err := fromLabelMatchers(req.Matchers)
	if err != nil {
		return 0, 0, nil, err
	}
	from := model.Time(req.StartTimestampMs)
	to := model.Time(req.EndTimestampMs)
	return from, to, matchers, nil
}

// ToQueryResult builds a QueryResult proto.
func ToQueryResult(sset storage.SeriesSet) (*prompb.QueryResult, error) {
	resp := &prompb.QueryResult{}

	for sset.Next() {
		si := sset.At().Iterator()
		if err := si.Err(); err != nil {
			return nil, err
		}

		ts := prompb.TimeSeries{
			Labels: ToLabelPairs(sset.At().Labels()),
		}
		for si.Next() {
			t, v := si.At()
			ts.Samples = append(ts.Samples, &prompb.Sample{
				Timestamp: t,
				Value:     v,
			})
		}
		if err := si.Err(); err != nil {
			return nil, err
		}

		resp.Timeseries = append(resp.Timeseries, &ts)
	}
	if err := sset.Err(); err != nil {
		return nil, err
	}
	return resp, nil
}

func fromLabelMatchers(matchers []*prompb.LabelMatcher) ([]*labels.Matcher, error) {
	result := make([]*labels.Matcher, 0, len(matchers))
	for _, matcher := range matchers {
		var mtype labels.MatchType
		switch matcher.Type {
		case prompb.LabelMatcher_EQ:
			mtype = labels.MatchEqual
		case prompb.LabelMatcher_NEQ:
			mtype = labels.MatchNotEqual
		case prompb.LabelMatcher_RE:
			mtype = labels.MatchRegexp
		case prompb.LabelMatcher_NRE:
			mtype = labels.MatchNotRegexp
		default:
			return nil, fmt.Errorf("invalid matcher type")
		}
		matcher, err := labels.NewMatcher(mtype, matcher.Name, matcher.Value)
		if err != nil {
			return nil, err
		}
		result = append(result, matcher)
	}
	return result, nil
}

func toLabelMatchers(matchers []*labels.Matcher) ([]*prompb.LabelMatcher, error) {
	result := make([]*prompb.LabelMatcher, 0, len(matchers))
	for _, matcher := range matchers {
		var mType prompb.LabelMatcher_Type
		switch matcher.Type {
		case labels.MatchEqual:
			mType = prompb.LabelMatcher_EQ
		case labels.MatchNotEqual:
			mType = prompb.LabelMatcher_NEQ
		case labels.MatchRegexp:
			mType = prompb.LabelMatcher_RE
		case labels.MatchNotRegexp:
			mType = prompb.LabelMatcher_NRE
		default:
			return nil, fmt.Errorf("invalid matcher type")
		}
		result = append(result, &prompb.LabelMatcher{
			Type:  mType,
			Name:  string(matcher.Name),
			Value: string(matcher.Value),
		})
	}
	return result, nil
}

// ToLabelPairs builds a []LabelPair from a model.Metric
func ToLabelPairs(lbls labels.Labels) []*prompb.Label {
	labels := make([]*prompb.Label, 0, len(lbls))
	for _, l := range lbls {
		labels = append(labels, &prompb.Label{
			Name:  l.Name,
			Value: l.Value,
		})
	}
	return labels
}

// FromLabelPairs unpack a []LabelPair to a model.Metric
func FromLabelPairs(lbls []*prompb.Label) model.Metric {
	metric := make(model.Metric, len(lbls))
	for _, l := range lbls {
		metric[model.LabelName(l.Name)] = model.LabelValue(l.Value)
	}
	return metric
}

// £ND FIXME
