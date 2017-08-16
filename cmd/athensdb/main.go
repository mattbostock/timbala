package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	log "github.com/Sirupsen/logrus"
	v1API "github.com/mattbostock/athensdb/internal/api/v1"
	"github.com/mattbostock/athensdb/internal/cluster"
	"github.com/mattbostock/athensdb/internal/write"
	"github.com/prometheus/common/route"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/tsdb"
	"golang.org/x/net/context"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	defaultHTTPAddr = "localhost:9080"
	defaultPeerAddr = "localhost:7946"

	apiRoute = "/api/v1"
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

	write.SetLogger(log.StandardLogger())
	write.SetStore(localStorage)
	router.Post(write.Route, write.Handler)

	cluster.SetLogger(log.StandardLogger())
	if err := cluster.Join(&cluster.Config{
		HTTPAdvertiseAddr: *config.httpAdvertiseAddr,
		HTTPBindAddr:      *config.httpBindAddr,
		PeerAdvertiseAddr: *config.peerAdvertiseAddr,
		PeerBindAddr:      *config.peerBindAddr,
		Peers:             config.peers,
	}); err != nil {
		log.Fatal("Failed to join the cluster: ", err)
	}

	log.Infof("Starting AthensDB node %s", cluster.LocalNode())
	log.Infof("Binding to %s for peer gossip; %s for HTTP", config.peerBindAddr, config.httpBindAddr)
	log.Infof("Advertising to cluster as %s for peer gossip; %s for HTTP", config.peerAdvertiseAddr, config.httpAdvertiseAddr)
	log.Infof("%d nodes in cluster: %s", len(cluster.Nodes()), cluster.Nodes())
	log.Fatal(http.ListenAndServe(config.httpBindAddr.String(), router))
}

func logFlagFatal(v ...interface{}) {
	log.Fatalf("%s: error: %s", os.Args[0], fmt.Sprint(v...))
}
