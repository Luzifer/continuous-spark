package main

import (
	"sync"
	"time"

	influx "github.com/influxdata/influxdb/client/v2"
	"github.com/pkg/errors"
)

const (
	influxWriteInterval = 10 * time.Second
)

type metricsSender struct {
	batch     influx.BatchPoints
	batchLock sync.Mutex
	client    influx.Client
	errs      chan error

	influxDB string
}

func newMetricsSender(influxHost, influxUser, influxPass, influxDatabase string) (*metricsSender, error) {
	out := &metricsSender{
		errs:     make(chan error, 10),
		influxDB: influxDatabase,
	}
	return out, out.initialize(influxHost, influxUser, influxPass)
}

func (m *metricsSender) Errors() <-chan error {
	return m.errs
}

func (m *metricsSender) ForceTransmit() error {
	return errors.Wrap(m.transmit(), "Unable to transmit recorded points")
}

func (m *metricsSender) RecordPoint(name string, tags map[string]string, fields map[string]interface{}) error {
	pt, err := influx.NewPoint(name, tags, fields, time.Now())
	if err != nil {
		return errors.Wrap(err, "Unable to create point")
	}

	m.batchLock.Lock()
	defer m.batchLock.Unlock()
	m.batch.AddPoint(pt)

	return nil
}

func (m *metricsSender) resetBatch() error {
	b, err := influx.NewBatchPoints(influx.BatchPointsConfig{
		Database: m.influxDB,
	})

	if err != nil {
		return errors.Wrap(err, "Unable to create new points batch")
	}

	m.batch = b
	return nil
}

func (m *metricsSender) sendLoop() {
	for range time.Tick(influxWriteInterval) {

		if err := m.transmit(); err != nil {
			m.errs <- err
		}

	}
}

func (m *metricsSender) transmit() error {
	m.batchLock.Lock()
	defer m.batchLock.Unlock()

	if err := m.client.Write(m.batch); err != nil {
		return errors.Wrap(err, "Unable to write recorded points")
	}
	return errors.Wrap(m.resetBatch(), "Unable to reset batch")
}

func (m *metricsSender) initialize(influxHost, influxUser, influxPass string) error {
	influxClient, err := influx.NewHTTPClient(influx.HTTPConfig{
		Addr:     influxHost,
		Username: influxUser,
		Password: influxPass,
		Timeout:  2 * time.Second,
	})

	if err != nil {
		return errors.Wrap(err, "Unable to create InfluxDB HTTP client")
	}

	m.client = influxClient
	if err := m.resetBatch(); err != nil {
		return errors.Wrap(err, "Unable to reset batch")
	}
	go m.sendLoop()

	return nil
}
