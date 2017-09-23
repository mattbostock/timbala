package main

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	v1API "github.com/mattbostock/athensdb/internal/api/v1"
	"github.com/mattbostock/athensdb/internal/cluster"
	"github.com/mattbostock/athensdb/internal/write"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/route"
	"github.com/prometheus/prometheus/promql"
	promtsdb "github.com/prometheus/prometheus/storage/tsdb"
	"github.com/prometheus/tsdb"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	applicationName = "athensdb"

	defaultDataDir  = "./data"
	defaultHTTPAddr = "localhost:9080"
	defaultPeerAddr = "localhost:7946"

	apiRoute     = "/api/v1"
	metricsRoute = "/metrics"
)

var (
	buildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: applicationName,
			Name:      "build_info",
			Help:      fmt.Sprintf("A metric with a constant '1' value labeled by the application's semantic version number"),
		},
		[]string{"version"},
	)

	config struct {
		dataDir           string
		httpAdvertiseAddr *net.TCPAddr
		httpBindAddr      *net.TCPAddr
		peerAdvertiseAddr *net.TCPAddr
		peerBindAddr      *net.TCPAddr
		peers             []string
	}
	version = "undefined"
)

func init() {
	buildInfo.WithLabelValues(version).Set(1)
	prometheus.MustRegister(buildInfo)
}

func main() {
	kingpin.Flag(
		"data-directory",
		"path to the directory to store data",
	).Default(defaultDataDir).StringVar(&config.dataDir)

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
		logFlagFatal("must specify host or IP for --http-advertise-addr")
	}
	if config.peerAdvertiseAddr.IP == nil || config.peerAdvertiseAddr.IP.IsUnspecified() {
		logFlagFatal("must specify host or IP for --peer-advertise-addr")
	}

	lvl, err := log.ParseLevel(*level)
	if err != nil {
		kingpin.Fatalf("could not parse log level %q", *level)
	}
	log.SetLevel(lvl)
	log.SetFormatter(&log.JSONFormatter{})

	// FIXME: Set logger
	localStorage, err := tsdb.Open(config.dataDir, nil, prometheus.DefaultRegisterer, &tsdb.Options{
		WALFlushInterval:  5 * time.Second,
		RetentionDuration: math.MaxUint64, // approximately 292,471,208 years
		BlockRanges:       tsdb.ExponentialBlockRanges(int64(2*time.Hour)/1e6, 3, 5),
		NoLockfile:        false,
	})
	if err != nil {
		log.Fatalf("Opening storage failed: %s", err)
	}

	var (
		_, cancelCtx = context.WithCancel(context.Background())
		queryEngine  = promql.NewEngine(promtsdb.Adapter(localStorage), promql.DefaultEngineOptions)
	)
	defer cancelCtx()

	// FIXME: Set context
	router := route.New()
	// Add debug endpoints manually; there's no easy way with this HTTP router library
	dbg := router.WithPrefix("/debug/pprof")
	dbg.Get("/", pprof.Index)
	dbg.Get("/cmdline", pprof.Cmdline)
	dbg.Get("/profile", pprof.Profile)
	dbg.Get("/symbol", pprof.Symbol)
	dbg.Post("/symbol", pprof.Symbol)
	dbg.Get("/trace", pprof.Trace)
	dbg.Get("/block", pprof.Handler("block").ServeHTTP)
	dbg.Get("/goroutine", pprof.Handler("goroutine").ServeHTTP)
	dbg.Get("/heap", pprof.Handler("heap").ServeHTTP)
	dbg.Get("/mutex", pprof.Handler("mutex").ServeHTTP)
	dbg.Get("/threadcreate", pprof.Handler("threadcreate").ServeHTTP)
	dbg.Get("/enable/block/:rate", func(w http.ResponseWriter, r *http.Request) {
		rate, err := strconv.Atoi(route.Param(r.Context(), "rate"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		runtime.SetBlockProfileRate(rate)
		fmt.Fprintf(w, "Block profile rate set to %d", rate)
	})
	dbg.Get("/enable/mutex/:fraction", func(w http.ResponseWriter, r *http.Request) {
		fraction, err := strconv.Atoi(route.Param(r.Context(), "fraction"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		prevValue := runtime.SetMutexProfileFraction(fraction)
		fmt.Fprintf(w, "Mutex profile fraction set to %d, previous value was: %d", fraction, prevValue)
	})

	var api = v1API.NewAPI(queryEngine, promtsdb.Adapter(localStorage))
	api.Register(router.WithPrefix(apiRoute))

	clstr, err := cluster.New(
		&cluster.Config{
			HTTPAdvertiseAddr: *config.httpAdvertiseAddr,
			HTTPBindAddr:      *config.httpBindAddr,
			PeerAdvertiseAddr: *config.peerAdvertiseAddr,
			PeerBindAddr:      *config.peerBindAddr,
			Peers:             config.peers,
		},
		log.StandardLogger(),
	)
	if err != nil {
		log.Fatal("Failed to join the cluster: ", err)
	}

	writer := write.New(clstr, log.StandardLogger(), promtsdb.Adapter(localStorage))
	router.Post(write.Route, writer.Handler)
	router.Get(metricsRoute, promhttp.Handler().ServeHTTP)

	absoluteDataDir, _ := filepath.Abs(config.dataDir)
	log.Infof("Starting AthensDB node %s; data will be stored in %s", clstr.LocalNode(), absoluteDataDir)
	log.Infof("Binding to %s for peer gossip; %s for HTTP", config.peerBindAddr, config.httpBindAddr)
	log.Infof("Advertising to cluster as %s for peer gossip; %s for HTTP", config.peerAdvertiseAddr, config.httpAdvertiseAddr)
	log.Infof("%d nodes in cluster: %s", len(clstr.Nodes()), clstr.Nodes())
	log.Fatal(http.ListenAndServe(config.httpBindAddr.String(), router))
}

func logFlagFatal(v ...interface{}) {
	log.Fatalf("%s: error: %s", os.Args[0], fmt.Sprint(v...))
}
