package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
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
	"golang.org/x/net/context/ctxhttp"
)

const (
	apiRoute   = "/api/v1"
	writeRoute = "/receive"

	httpHeaderInternalWrite = "X-AthensDB-Internal-Write-Version"
)

var (
	config = struct {
		listenAddr string
		peerAddr   string
		peers      []string
	}{
		":9080",
		":7946", // default used by memberlist
		[]string{},
	}
	version = "undefined"
)

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
	var v bool
	flag.BoolVar(&v, "version", false, "prints current version")
	flag.Parse()

	if v {
		fmt.Println(version)
		os.Exit(0)
	}

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
		nodes := list.Members()
		var wgErrChan = make(chan error, len(nodes))
		for _, node := range nodes {
			if node.String() == list.LocalNode().String() {
				log.Debugf("Skipping local node %s", node)
				continue
			}

			wg.Add(1)
			go func(n *memberlist.Node) {
				defer wg.Done()

				log.Debugf("Writing %d samples to %s", samples, n)

				// FIXME Remove hardcoded port, use advertised host from shared state
				nodeReq, err := http.NewRequest("POST", "http://"+n.String()+":9080"+writeRoute, bytes.NewBuffer(compressed))
				if err != nil {
					wgErrChan <- err
					fmt.Println(err)
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
					fmt.Println(err)
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

	_, err = list.Join(config.peers)
	if err != nil {
		log.Fatal("Failed to join the cluster: ", err)
	}

	// FIXME catch errors
	var errChan chan error
	go func() {
		errChan <- http.ListenAndServe(config.listenAddr, router)
	}()

	members := list.Members()
	log.Infof("Starting AthensDB node %s; peer address %s; API address %s", list.LocalNode(), config.peerAddr, config.listenAddr)
	log.Infof("%d nodes in cluster: %s", len(members), members)

	log.Fatal(<-errChan)
}
