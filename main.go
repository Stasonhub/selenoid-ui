package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/aerokube/selenoid-ui/selenoid"
	"github.com/aerokube/selenoid-ui/sse"
	"github.com/koding/websocketproxy"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"net/url"
	"strings"
)

//go:generate go-bindata-assetfs data/...

var (
	listen      string
	selenoidUri string
	period      time.Duration

	version     bool
	gitRevision string = "HEAD"
	buildStamp  string = "unknown"
)

func mux(sse *sse.SseBroker) http.Handler {
	mux := http.NewServeMux()
	static := http.FileServer(assetFS())

	mux.Handle("/", static)
	mux.Handle("/events", sse)

	parsedUri, err := url.Parse(selenoidUri)
	if err != nil {
		log.Fatal(err)
	}

	mux.HandleFunc("/ws/", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/ws")
		ws := &url.URL{Scheme: "ws", Host: parsedUri.Host, Path: r.URL.Path}
		log.Printf("[WS_PROXY] [/ws%s] [%s]", r.URL.Path, ws)
		websocketproxy.NewProxy(ws).ServeHTTP(w, r)
		log.Printf("[WS_PROXY] [%s] [CLOSED]", r.URL.Path)
	})
	return mux
}

func showVersion() {
	fmt.Printf("Git Revision: %s\n", gitRevision)
	fmt.Printf("UTC Build Time: %s\n", buildStamp)
}

func init() {
	flag.StringVar(&listen, "listen", ":8080", "host and port to listen on")
	flag.StringVar(&selenoidUri, "selenoid-uri", "http://localhost:4444", "selenoid uri to fetch data from")
	flag.DurationVar(&period, "period", 5*time.Second, "data refresh period, e.g. 5s or 1m")
	flag.BoolVar(&version, "version", false, "Show version and exit")
	flag.Parse()

	if version {
		showVersion()
		os.Exit(0)
	}
}

func tick(broker sse.Broker, selenoidUri string, period time.Duration, stop chan os.Signal) {
	ticker := time.NewTicker(period)
	for {
		ctx, cancel := context.WithCancel(context.Background())
		select {
		case <-ticker.C:
			{
				if broker.HasClients() {
					res, err := selenoid.Status(ctx, selenoidUri)
					if err != nil {
						log.Printf("can't get status (%s)\n", err)
						broker.Notify([]byte(`{ "errors": [{"msg": "can't get status"}] }`))
						continue
					}
					broker.Notify(res)
				}
			}
		case <-stop:
			{
				cancel()
				ticker.Stop()
				os.Exit(0)
			}
		}
	}
}

func main() {
	broker := sse.NewSseBroker()
	stop := make(chan os.Signal)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)

	go tick(broker, selenoidUri, period, stop)

	log.Printf("Listening on %s\n", listen)
	log.Fatal(http.ListenAndServe(listen, mux(broker)))
}
