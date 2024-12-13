package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	bitsPerByte         = 8
	throughputChunkSize = 1024 * blockSize
)

func (s *sparkClient) ExecuteThroughputTest(t *testResult) (err error) {
	if err = s.runSendTest(t); err != nil {
		return fmt.Errorf("running send-test: %w", err)
	}

	if err = s.runRecvTest(t); err != nil {
		return fmt.Errorf("running recv-test: %w", err)
	}

	return nil
}

func (s *sparkClient) runSendTest(t *testResult) (err error) {
	if err = s.connect(); err != nil {
		return fmt.Errorf("establishing connection: %w", err)
	}
	defer func() {
		if err := s.conn.Close(); err != nil {
			logrus.WithError(err).Error("closing connection (leaked fd)")
		}
	}()

	if err = s.writeCommand("RCV"); err != nil {
		return fmt.Errorf("sending RCV command: %w", err)
	}

	if t.Send.Min, t.Send.Max, t.Send.Avg, err = s.runThroughputTest(rand.Reader, s.conn); err != nil {
		return fmt.Errorf("testing throughput: %w", err)
	}

	return nil
}

func (s *sparkClient) runRecvTest(t *testResult) (err error) {
	if err = s.connect(); err != nil {
		return fmt.Errorf("establishing connection: %w", err)
	}
	defer func() {
		if err := s.conn.Close(); err != nil {
			logrus.WithError(err).Error("closing connection (leaked fd)")
		}
	}()

	if err = s.writeCommand("SND"); err != nil {
		return fmt.Errorf("writing SND command: %w", err)
	}

	if t.Receive.Min, t.Receive.Max, t.Receive.Avg, err = s.runThroughputTest(s.conn, io.Discard); err != nil {
		return fmt.Errorf("testing throughput: %w", err)
	}

	return nil
}

func (*sparkClient) runThroughputTest(src io.Reader, dst io.Writer) (minT, maxT, avgT float64, err error) {
	var (
		dataTxBytes int64
		testStart   = time.Now()
	)

	for {
		segmentStart := time.Now()

		n, err := io.CopyN(dst, src, throughputChunkSize)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("copying data: %w", err)
		}

		dataTxBytes += n

		bps := float64(n*bitsPerByte) / float64(time.Since(segmentStart).Seconds())
		if bps < minT {
			minT = bps
		}
		if bps > maxT {
			maxT = bps
		}

		if time.Since(testStart) > throughputTestLength {
			break
		}
	}

	// average bit per second
	avgT = float64(dataTxBytes*bitsPerByte) / float64(time.Since(testStart).Seconds())

	return minT, maxT, avgT, nil
}
