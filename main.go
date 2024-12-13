package main

import (
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/Luzifer/rconfig/v2"
)

const tsvPermission = 0o600

var (
	cfg struct {
		InfluxDB       string        `flag:"influx-db" default:"" description:"Name of the database to write to (if unset, InfluxDB feature is disabled)"`
		InfluxHost     string        `flag:"influx-host" default:"http://localhost:8086" description:"Host with protocol of the InfluxDB"`
		InfluxPass     string        `flag:"influx-pass" default:"" description:"Password for the InfluxDB user"`
		InfluxUser     string        `flag:"influx-user" default:"" description:"Username for the InfluxDB connection"`
		Interface      string        `flag:"interface" default:"" description:"Bind to interface for testing a specific interface throughput"`
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

func initApp() (err error) {
	rconfig.AutoEnv(true)
	if err = rconfig.ParseAndValidate(&cfg); err != nil {
		return fmt.Errorf("parsing CLI params: %w", err)
	}

	l, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("parsing log-level: %w", err)
	}
	logrus.SetLevel(l)

	return nil
}

func main() {
	var err error
	if err = initApp(); err != nil {
		logrus.WithError(err).Fatal("initializing app")
	}

	if cfg.VersionAndExit {
		fmt.Printf("continuous-spark %s\n", version) //nolint:forbidigo
		os.Exit(0)
	}

	if cfg.InfluxDB != "" {
		if metrics, err = newMetricsSender(cfg.InfluxHost, cfg.InfluxUser, cfg.InfluxPass, cfg.InfluxDB); err != nil {
			logrus.WithError(err).Fatalf("initializing InfluxDB sender")
		}

		go func() {
			for err := range metrics.Errors() {
				logrus.WithError(err).Error("transmitting metrics")
			}
		}()
	}

	if err := updateStats(execTest()); err != nil {
		logrus.WithError(err).Error("updating stats")
	}

	if cfg.OneShot {
		// Return before loop for oneshot execution
		if metrics != nil {
			if err := metrics.ForceTransmit(); err != nil {
				logrus.WithError(err).Error("storing metrics")
			}
		}
		return
	}

	for range time.Tick(cfg.Interval) {
		if err := updateStats(execTest()); err != nil {
			logrus.WithError(err).Error("updating stats")
		}
	}
}

func updateStats(t *testResult, err error) error {
	if err != nil {
		return err
	}

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("getting hostname: %w", err)
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
			return fmt.Errorf("recording ping-metric: %w", err)
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
			return fmt.Errorf("recording down-metric: %w", err)
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
			return fmt.Errorf("recording up-metric: %w", err)
		}
	}

	if cfg.TSVFile != "" {
		if err := writeTSV(t); err != nil {
			return fmt.Errorf("writing TSV file: %w", err)
		}
	}

	return nil
}

func writeTSV(t *testResult) (err error) {
	if _, err = os.Stat(cfg.TSVFile); err != nil && os.IsNotExist(err) {
		if err = os.WriteFile(cfg.TSVFile, []byte("Date\tPing Min (ms)\tPing Avg (ms)\tPing Max (ms)\tPing StdDev (ms)\tRX Avg (bps)\tTX Avg (bps)\n"), tsvPermission); err != nil {
			return fmt.Errorf("writing TSV headers: %w", err)
		}
	}

	f, err := os.OpenFile(cfg.TSVFile, os.O_APPEND|os.O_WRONLY, os.ModeAppend)
	if err != nil {
		return fmt.Errorf("opening TSV file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			logrus.WithError(err).Error("closing TSV file (leaked fd)")
		}
	}()

	if _, err = fmt.Fprintf(f, "%s\t%.2f\t%.2f\t%.2f\t%.2f\t%.0f\t%.0f\n",
		time.Now().Format(time.RFC3339),
		t.Ping.Min,
		t.Ping.Avg,
		t.Ping.Max,
		t.Ping.Dev,
		t.Receive.Avg,
		t.Send.Avg,
	); err != nil {
		return fmt.Errorf("writing measurement: %w", err)
	}

	return nil
}

func execTest() (*testResult, error) {
	t := newTestResult()

	sc := newSparkClient(cfg.Server, cfg.Port, cfg.Interface)
	if err := sc.ExecutePingTest(t); err != nil {
		return nil, fmt.Errorf("executing ping-test: %w", err)
	}

	if err := sc.ExecuteThroughputTest(t); err != nil {
		return nil, fmt.Errorf("executing throughput-test: %w", err)
	}

	logrus.Debugf("%s", t)
	return t, nil
}
