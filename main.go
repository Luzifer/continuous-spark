package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/Luzifer/rconfig"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var (
	cfg struct {
		Hostname       string        `flag:"hostname" default:"" description:"Hostname / IP of the sparkyfish server" validate:"nonzero"`
		Interval       time.Duration `flag:"interval" default:"15m" description:"Interval to execute test in"`
		Listen         string        `flag:"listen" default:":3000" description:"IP/Port to listen on"`
		LogLevel       string        `flag:"log-level" default:"info" description:"Set log level (debug, info, warning, error)"`
		Port           int           `flag:"port" default:"7121" description:"Port the sparkyfish server is running on"`
		VersionAndExit bool          `flag:"version" default:"false" description:"Print version information and exit"`
	}

	version = "dev"
)

func init() {
	if err := rconfig.ParseAndValidate(&cfg); err != nil {
		log.Fatalf("Unable to parse CLI parameters: %s", err)
	}

	if cfg.VersionAndExit {
		fmt.Printf("continuous-spark %s\n", version)
		os.Exit(0)
	}

	if l, err := log.ParseLevel(cfg.LogLevel); err == nil {
		log.SetLevel(l)
	} else {
		log.Fatalf("Invalid log level: %s", err)
	}
}

func main() {
	go func() {
		if err := updateStats(execTest()); err != nil {
			log.Error(err.Error())
		}

		for range time.Tick(cfg.Interval) {
			if err := updateStats(execTest()); err != nil {
				log.Error(err.Error())
				continue
			}
		}
	}()

	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(cfg.Listen, nil)
}

func updateStats(t *testResult, err error) error {
	if err != nil {
		return err
	}

	pingAvg.Set(t.Ping.Avg)
	thresholdAvg.WithLabelValues("recv").Set(t.Receive.Avg)
	thresholdAvg.WithLabelValues("send").Set(t.Send.Avg)

	return nil
}

func execTest() (*testResult, error) {
	t := newTestResult()

	sc := newSparkClient(cfg.Hostname, cfg.Port)
	if err := sc.ExecutePingTest(t); err != nil {
		return nil, fmt.Errorf("Ping test fucked up: %s", err)
	}

	if err := sc.ExecuteThroughputTest(t); err != nil {
		return nil, fmt.Errorf("Throughput test fucked up: %s", err)
	}

	log.Debugf("%s", t)
	return t, nil
}
