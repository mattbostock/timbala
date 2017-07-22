package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	v1API "github.com/mattbostock/athensdb/internal/api/v1"
	"github.com/mattbostock/athensdb/internal/cluster"
	"github.com/mattbostock/athensdb/internal/remote"
	"github.com/prometheus/common/route"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/tsdb"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	defaultHTTPAddr = "localhost:9080"
	defaultPeerAddr = "localhost:7946"

	apiRoute   = "/api/v1"
	writeRoute = "/receive"

	httpHeaderInternalWrite = "X-AthensDB-Internal-Write-Version"
)

var (
	config struct {
		httpAdvertiseAddr *net.TCPAddr
		httpBindAddr      *net.TCPAddr
		peerAdvertiseAddr *net.TCPAddr
		peerBindAddr      *net.TCPAddr
		peers             []string
	}
	version = "undefined"
)

func main() {
	kingpin.Flag(
		"http-advertise-addr",
		"host:port to advertise to other nodes for HTTP",
	).Default(defaultHTTPAddr).TCPVar(&config.httpAdvertiseAddr)

	kingpin.Flag(
		"http-bind-addr",
		"host:port to bind to for HTTP",
	).Default(defaultHTTPAddr).TCPVar(&config.httpBindAddr)

	kingpin.Flag(
		"peer-advertise-addr",
		"host:port to advertise to other nodes for cluster communication",
	).Default(defaultPeerAddr).TCPVar(&config.peerAdvertiseAddr)

	kingpin.Flag(
		"peer-bind-addr",
		"host:port to bind to for cluster communication",
	).Default(defaultPeerAddr).TCPVar(&config.peerBindAddr)

	kingpin.Flag(
		"peers",
		"List of peers to connect to",
	).StringsVar(&config.peers)

	level := kingpin.Flag(
		"log-level",
		"Log level",
	).Default(log.InfoLevel.String()).Enum("debug", "info", "warn", "panic", "fatal")

	_, err := kingpin.Version(version).
		DefaultEnvars().
		Parse(os.Args[1:])
	if err != nil {
		logFlagFatal(err)
	}

	if config.httpAdvertiseAddr.IP == nil || config.httpAdvertiseAddr.IP.IsUnspecified() {
		logFlagFatal("must specify host or IP for --http--advertise-addr")
	}
	if config.peerAdvertiseAddr.IP == nil || config.peerAdvertiseAddr.IP.IsUnspecified() {
		logFlagFatal("must specify host or IP for --peer--advertise-addr")
	}

	lvl, err := log.ParseLevel(*level)
	if err != nil {
		kingpin.Fatalf("could not parse log level %q", *level)
	}
	log.SetLevel(lvl)

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
		if err != nil {
			// FIXME: Make error more useful
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer appender.Commit()

		var samples uint32
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
				samples++
			}
		}

		// This is an internal write, so don't replicate it to other nodes
		if r.Header.Get(httpHeaderInternalWrite) != "" {
			log.Debugf("Received %d samples from another node in the cluster", samples)
			return
		}

		var wg sync.WaitGroup
		// FIXME: Avoid panic if the cluster is not yet initialised
		nodes := cluster.Nodes()
		var wgErrChan = make(chan error, len(nodes))
		for _, node := range nodes {
			if node.Name() == cluster.LocalNode().Name() {
				log.Debugf("Skipping local node %s", node)
				continue
			}

			wg.Add(1)
			go func(n *cluster.Node) {
				defer wg.Done()

				log.Debugf("Writing %d samples to %s", samples, n)

				httpAddr, err := n.HTTPAddr()
				if err != nil {
					wgErrChan <- err
					return
				}
				apiURL := fmt.Sprintf("%s%s%s", "http://", httpAddr, writeRoute)
				nodeReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(compressed))
				if err != nil {
					wgErrChan <- err
					return
				}
				nodeReq.Header.Add("Content-Encoding", "snappy")
				nodeReq.Header.Set("Content-Type", "application/x-protobuf")
				nodeReq.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")
				nodeReq.Header.Set(httpHeaderInternalWrite, "0.0.1")

				// FIXME set timeout using context
				httpResp, err := ctxhttp.Do(context.TODO(), http.DefaultClient, nodeReq)
				if err != nil {
					wgErrChan <- err
					return
				}
				defer httpResp.Body.Close()
			}(node)
		}
		// FIXME cancel requests if one fails
		wg.Wait()

		select {
		case err := <-wgErrChan:
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		default:
		}
	})

	if err := cluster.Join(&cluster.Config{
		HTTPAdvertiseAddr: *config.httpAdvertiseAddr,
		HTTPBindAddr:      *config.httpBindAddr,
		PeerAdvertiseAddr: *config.peerAdvertiseAddr,
		PeerBindAddr:      *config.peerBindAddr,
		Peers:             config.peers,
	}); err != nil {
		log.Fatal("Failed to join the cluster: ", err)
	}

	log.Infof("starting AthensDB node %s", cluster.LocalNode())
	log.Infof("binding to %s for peer gossip; %s for HTTP", config.peerBindAddr, config.httpBindAddr)
	log.Infof("advertising to cluster as %s for peer gossip; %s for HTTP", config.peerAdvertiseAddr, config.httpAdvertiseAddr)
	log.Infof("%d nodes in cluster: %s", len(cluster.Nodes()), cluster.Nodes())
	log.Fatal(http.ListenAndServe(config.httpBindAddr.String(), router))
}

func logFlagFatal(v ...interface{}) {
	log.Fatalf("%s: error: %s", os.Args[0], fmt.Sprint(v...))
}
