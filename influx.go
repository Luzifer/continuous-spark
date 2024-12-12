package main

import (
	"fmt"
	"sync"
	"time"

	influx "github.com/influxdata/influxdb/client/v2"
)

const (
	influxTimeout       = 2 * time.Second
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
		errs:     make(chan error, 1),
		influxDB: influxDatabase,
	}
	return out, out.initialize(influxHost, influxUser, influxPass)
}

func (m *metricsSender) Errors() <-chan error {
	return m.errs
}

func (m *metricsSender) ForceTransmit() (err error) {
	if err = m.transmit(); err != nil {
		return fmt.Errorf("transmitting recorded points: %w", err)
	}
	return nil
}

func (m *metricsSender) RecordPoint(name string, tags map[string]string, fields map[string]interface{}) error {
	pt, err := influx.NewPoint(name, tags, fields, time.Now())
	if err != nil {
		return fmt.Errorf("creating point: %w", err)
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
		return fmt.Errorf("creating points batch: %w", err)
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

func (m *metricsSender) transmit() (err error) {
	m.batchLock.Lock()
	defer m.batchLock.Unlock()

	if err = m.client.Write(m.batch); err != nil {
		return fmt.Errorf("writing recorded points: %w", err)
	}

	if err = m.resetBatch(); err != nil {
		return fmt.Errorf("resetting batch: %w", err)
	}

	return nil
}

func (m *metricsSender) initialize(influxHost, influxUser, influxPass string) error {
	influxClient, err := influx.NewHTTPClient(influx.HTTPConfig{
		Addr:     influxHost,
		Username: influxUser,
		Password: influxPass,
		Timeout:  influxTimeout,
	})
	if err != nil {
		return fmt.Errorf("creating InfluxDB client: %w", err)
	}

	m.client = influxClient
	if err = m.resetBatch(); err != nil {
		return fmt.Errorf("resetting batch: %w", err)
	}
	go m.sendLoop()

	return nil
}
