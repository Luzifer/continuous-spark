package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	throughputBufferSize     = 1024 * blockSize
	throughputBufferSizeBits = throughputBufferSize * 8
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

//nolint:gocyclo
func (s *sparkClient) runSendTest(t *testResult) (err error) {
	data := make([]byte, throughputBufferSize)
	if _, err = rand.Read(data); err != nil {
		return fmt.Errorf("gathering random data: %w", err)
	}
	dataReader := bytes.NewReader(data)

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

	var (
		blockCount int64
		totalStart = time.Now()
	)

	for {
		start := time.Now()

		if _, err = io.Copy(s.conn, dataReader); err != nil {
			// If we get any of these errors, it probably just means that the server closed the connection
			if err == io.EOF || err == io.ErrClosedPipe || err == syscall.EPIPE {
				break
			}

			if operr, ok := err.(*net.OpError); ok {
				logrus.Printf("%s", operr.Err)
			}

			if operr, ok := err.(*net.OpError); ok && operr.Err.Error() == syscall.ECONNRESET.Error() {
				break
			}

			return fmt.Errorf("copying data: %w", err)
		}

		bps := float64(throughputBufferSizeBits) / (float64(time.Since(start).Nanoseconds()) / float64(time.Second.Nanoseconds()))
		if bps < t.Send.Min {
			t.Send.Min = bps
		}
		if bps > t.Send.Max {
			t.Send.Max = bps
		}
		blockCount++

		if _, err := dataReader.Seek(0, 0); err != nil {
			return fmt.Errorf("seeking data reader: %w", err)
		}

		if time.Since(totalStart) > time.Duration(throughputTestLength)*time.Second {
			break
		}
	}

	// average bit per second
	t.Send.Avg = float64(throughputBufferSizeBits) / (float64(time.Since(totalStart).Nanoseconds()) / float64(time.Second.Nanoseconds()))

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

	var (
		blockCount int64
		totalStart = time.Now()
	)

	for {
		start := time.Now()

		if _, err = io.CopyN(io.Discard, s.conn, throughputBufferSize); err != nil {
			// If we get any of these errors, it probably just means that the server closed the connection
			if err == io.EOF || err == io.ErrClosedPipe || err == syscall.EPIPE {
				break
			}

			if operr, ok := err.(*net.OpError); ok && operr.Err.Error() == syscall.ECONNRESET.Error() {
				break
			}

			return fmt.Errorf("copying data: %w", err)
		}

		bps := float64(throughputBufferSizeBits) / (float64(time.Since(start).Nanoseconds()) / float64(time.Second.Nanoseconds()))
		if bps < t.Receive.Min {
			t.Receive.Min = bps
		}
		if bps > t.Receive.Max {
			t.Receive.Max = bps
		}
		blockCount++

		if time.Since(totalStart) > time.Duration(throughputTestLength)*time.Second {
			break
		}
	}

	// average bit per second
	t.Receive.Avg = float64(throughputBufferSizeBits) / (float64(time.Since(totalStart).Nanoseconds()) / float64(time.Second.Nanoseconds()))

	return nil
}
