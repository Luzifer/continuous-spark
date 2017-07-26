package main

import "github.com/prometheus/client_golang/prometheus"

var (
	pingAvg = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sparkyfish_ping_avg",
		Help: "Average ping of the test run (ms)",
	})
	thresholdAvg = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sparkyfish_threshold_avg",
		Help: "Average threshold of the test run (bps)",
	}, []string{"direction"})
)

func init() {
	prometheus.MustRegister(pingAvg)
	prometheus.MustRegister(thresholdAvg)
}
