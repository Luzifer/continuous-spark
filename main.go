package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/Luzifer/rconfig"
)

var (
	cfg struct {
		InfluxDB       string        `flag:"influx-db" default:"" description:"Name of the database to write to (if unset, InfluxDB feature is disabled)"`
		InfluxHost     string        `flag:"influx-host" default:"http://localhost:8086" description:"Host with protocol of the InfluxDB"`
		InfluxPass     string        `flag:"influx-pass" default:"" description:"Password for the InfluxDB user"`
		InfluxUser     string        `flag:"influx-user" default:"" description:"Username for the InfluxDB connection"`
		Interval       time.Duration `flag:"interval" default:"15m" description:"Interval to execute test in"`
		LogLevel       string        `flag:"log-level" default:"info" description:"Set log level (debug, info, warning, error)"`
		OneShot        bool          `flag:"oneshot,1" default:"false" description:"Execute one measurement and exit (for cron execution)"`
		Port           int           `flag:"port" default:"7121" description:"Port the sparkyfish server is running on"`
		Server         string        `flag:"server" default:"" description:"Hostname / IP of the sparkyfish server" validate:"nonzero"`
		TSVFile        string        `flag:"tsv-file" default:"measures.tsv" description:"File to write the results to (set to empty string to disable)"`
		VersionAndExit bool          `flag:"version" default:"false" description:"Print version information and exit"`
	}

	metrics *metricsSender

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
	var err error

	if cfg.InfluxDB != "" {
		if metrics, err = newMetricsSender(cfg.InfluxHost, cfg.InfluxUser, cfg.InfluxPass, cfg.InfluxDB); err != nil {
			log.WithError(err).Fatalf("Unable to initialize InfluxDB sender")
		}

		go func() {
			for err := range metrics.Errors() {
				log.WithError(err).Error("Unable to transmit metrics")
			}
		}()
	}

	if err := updateStats(execTest()); err != nil {
		log.WithError(err).Error("Unable to update stats")
	}

	if cfg.OneShot {
		// Return before loop for oneshot execution
		if metrics != nil {
			if err := metrics.ForceTransmit(); err != nil {
				log.WithError(err).Error("Unable to store metrics")
			}
		}
		return
	}

	for range time.Tick(cfg.Interval) {
		if err := updateStats(execTest()); err != nil {
			log.WithError(err).Error("Unable to update stats")
		}
	}
}

func updateStats(t *testResult, err error) error {
	if err != nil {
		return errors.Wrap(err, "Got error from test function")
	}

	hostname, err := os.Hostname()
	if err != nil {
		return errors.Wrap(err, "Unable to get local hostname")
	}

	if metrics != nil {
		if err := metrics.RecordPoint(
			"sparkyfish_ping",
			map[string]string{
				"hostname": hostname,
				"server":   cfg.Server,
			},
			map[string]interface{}{
				"avg": t.Ping.Avg,
				"min": t.Ping.Min,
				"max": t.Ping.Max,
				"dev": t.Ping.Dev,
			},
		); err != nil {
			return errors.Wrap(err, "Unable to record 'ping' metric")
		}

		if err := metrics.RecordPoint(
			"sparkyfish_transfer",
			map[string]string{
				"direction": "down",
				"hostname":  hostname,
				"server":    cfg.Server,
			},
			map[string]interface{}{
				"avg": t.Receive.Avg,
				"min": t.Receive.Min,
				"max": t.Receive.Max,
			},
		); err != nil {
			return errors.Wrap(err, "Unable to record 'down' metric")
		}

		if err := metrics.RecordPoint(
			"sparkyfish_transfer",
			map[string]string{
				"direction": "up",
				"hostname":  hostname,
				"server":    cfg.Server,
			},
			map[string]interface{}{
				"avg": t.Send.Avg,
				"min": t.Send.Min,
				"max": t.Send.Max,
			},
		); err != nil {
			return errors.Wrap(err, "Unable to record 'up' metric")
		}
	}

	if cfg.TSVFile != "" {
		if err := writeTSV(t); err != nil {
			return errors.Wrap(err, "Unable to write TSV file")
		}
	}

	return nil
}

func writeTSV(t *testResult) error {
	if _, err := os.Stat(cfg.TSVFile); err != nil && os.IsNotExist(err) {
		if err := ioutil.WriteFile(cfg.TSVFile, []byte("Date\tPing Min (ms)\tPing Avg (ms)\tPing Max (ms)\tPing StdDev (ms)\tRX Avg (bps)\tTX Avg (bps)\n"), 0o644); err != nil {
			return errors.Wrap(err, "Unable to write initial TSV headers")
		}
	}

	f, err := os.OpenFile(cfg.TSVFile, os.O_APPEND|os.O_WRONLY, os.ModeAppend)
	if err != nil {
		return errors.Wrap(err, "Unable to open TSV file")
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

	return errors.Wrap(err, "Unable to write measurement to TSV file")
}

func execTest() (*testResult, error) {
	t := newTestResult()

	sc := newSparkClient(cfg.Server, cfg.Port)
	if err := sc.ExecutePingTest(t); err != nil {
		return nil, errors.Wrap(err, "Ping-test failed")
	}

	if err := sc.ExecuteThroughputTest(t); err != nil {
		return nil, errors.Wrap(err, "Throughput test failed")
	}

	log.Debugf("%s", t)
	return t, nil
}
