package main

import (
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/hashicorp/memberlist"
	v1API "github.com/mattbostock/athensdb/api/v1"
	"github.com/mattbostock/athensdb/remote"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/route"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/tsdb"
	"golang.org/x/net/context"
)

const (
	apiRoute   = "/api/v1"
	writeRoute = "/receive"
)

var config = struct {
	listenAddr string
	peerAddr   string
	peers      []string
}{
	":9080",
	":7946", // default used by memberlist
	[]string{},
}

func init() {
	if len(os.Getenv("ADDR")) > 0 {
		config.listenAddr = os.Getenv("ADDR")
	}

	if len(os.Getenv("PEER_ADDR")) > 0 {
		config.peerAddr = os.Getenv("PEER_ADDR")
	}

	if len(os.Getenv("PEERS")) > 0 {
		config.peers = strings.Split(os.Getenv("PEERS"), ",")
		for k, v := range config.peers {
			config.peers[k] = strings.TrimSpace(v)
		}
	}
}

func main() {
	// FIXME(mbostock): Consider using a non-local config for memberlist
	memberConf := memberlist.DefaultLocalConfig()
	peerHost, peerPort, _ := net.SplitHostPort(config.peerAddr)
	bindPort, _ := strconv.Atoi(peerPort)
	memberConf.BindAddr = peerHost
	memberConf.BindPort = bindPort
	memberConf.LogOutput = ioutil.Discard

	list, err := memberlist.Create(memberConf)
	if err != nil {
		log.Fatal("Failed to configure cluster settings:", err)
	}

	_, err = list.Join(config.peers)
	if err != nil {
		log.Fatal("Failed to join the cluster: ", err)
	}

	members := list.Members()
	log.Infof("Starting AthensDB node %s; peer address %s; API address %s", list.LocalNode(), config.peerAddr, config.listenAddr)
	log.Infof("%d nodes in cluster: %s", len(members), members)

	localStorage, err := tsdb.Open("data", nil, &tsdb.Options{
		AppendableBlocks: 2,
		MinBlockDuration: 2 * time.Hour,
		MaxBlockDuration: 36 * time.Hour,
		Retention:        15 * 24 * time.Hour,
	})
	if err != nil {
		log.Fatalf("Opening storage failed: %s", err)
	}

	var (
		ctx, cancelCtx = context.WithCancel(context.Background())
		queryEngine    = promql.NewEngine(localStorage, promql.DefaultEngineOptions)
	)
	defer cancelCtx()

	router := route.New(func(r *http.Request) (context.Context, error) {
		return ctx, nil
	})

	var api = v1API.NewAPI(queryEngine, localStorage)
	api.Register(router.WithPrefix(apiRoute))

	router.Post(writeRoute, func(w http.ResponseWriter, r *http.Request) {
		compressed, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		reqBuf, err := snappy.Decode(nil, compressed)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var req remote.WriteRequest
		if err := proto.Unmarshal(reqBuf, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		appender, err := localStorage.Appender()
		defer appender.Commit()

		if err != nil {
			// FIXME: Make error more useful
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		for _, ts := range req.Timeseries {
			m := make(labels.Labels, 0, len(ts.Labels))
			for _, l := range ts.Labels {
				m = append(m, labels.Label{
					Name:  l.Name,
					Value: l.Value,
				})
			}
			sort.Sort(m)

			for _, s := range ts.Samples {
				// FIXME: Look at using AddFast
				appender.Add(m, s.TimestampMs, s.Value)
			}
		}
	})

	log.Fatal(http.ListenAndServe(config.listenAddr, router))
}
