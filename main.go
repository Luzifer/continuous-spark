package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/Luzifer/rconfig"
	log "github.com/sirupsen/logrus"
	stathat "github.com/stathat/go"
)

const (
	metricPing        = "[CS] Ping"
	metricThresholdRX = "[CS] Threshold RX"
	metricThresholdTX = "[CS] Threshold TX"
)

var (
	cfg struct {
		Hostname       string        `flag:"hostname" default:"" description:"Hostname / IP of the sparkyfish server" validate:"nonzero"`
		Interval       time.Duration `flag:"interval" default:"15m" description:"Interval to execute test in"`
		LogLevel       string        `flag:"log-level" default:"info" description:"Set log level (debug, info, warning, error)"`
		StatHatEZKey   string        `flag:"stathat-ezkey" default:"" description:"Key to post metrics to" validate:"nonzero"`
		Port           int           `flag:"port" default:"7121" description:"Port the sparkyfish server is running on"`
		TSVFile        string        `flag:"tsv-file" default:"measures.tsv" description:"File to write the results to"`
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
	if err := updateStats(execTest()); err != nil {
		log.Error(err.Error())
	}

	for range time.Tick(cfg.Interval) {
		if err := updateStats(execTest()); err != nil {
			log.Error(err.Error())
			continue
		}
	}
}

func updateStats(t *testResult, err error) error {
	if err != nil {
		return err
	}

	stathat.PostEZValue(metricPing, cfg.StatHatEZKey, t.Ping.Avg)
	stathat.PostEZValue(metricThresholdRX, cfg.StatHatEZKey, t.Receive.Avg)
	stathat.PostEZValue(metricThresholdTX, cfg.StatHatEZKey, t.Send.Avg)

	return writeTSV(t)
}

func writeTSV(t *testResult) error {
	if _, err := os.Stat(cfg.TSVFile); err != nil && os.IsNotExist(err) {
		if err := ioutil.WriteFile(cfg.TSVFile, []byte("Date\tPing Min (ms)\tPing Avg (ms)\tPing Max (ms)\tPing StdDev (ms)\tRX Avg (bps)\tTX Avg (bps)\n"), 0644); err != nil {
			return err
		}
	}

	f, err := os.OpenFile(cfg.TSVFile, os.O_APPEND|os.O_WRONLY, os.ModeAppend)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = fmt.Fprintf(f, "%s\t%.2f\t%.2f\t%.2f\t%.2f\t%.0f\t%.0f\n",
		time.Now().Format(time.RFC3339),
		t.Ping.Min,
		t.Ping.Avg,
		t.Ping.Max,
		t.Ping.Dev,
		t.Receive.Avg,
		t.Send.Avg,
	)

	return err
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
