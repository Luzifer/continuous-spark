package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"syscall"
	"time"
)

func (s *sparkClient) ExecuteThroughputTest(t *testResult) error {
	if err := s.runSendTest(t); err != nil {
		return err
	}
	return s.runRecvTest(t)
}

func (s *sparkClient) runSendTest(t *testResult) error {
	data := make([]byte, 1024*blockSize)
	if _, err := rand.Read(data); err != nil {
		return fmt.Errorf("Was unable to gather random data: %s", err)
	}
	dataReader := bytes.NewReader(data)

	if err := s.connect(); err != nil {
		return err
	}
	defer s.conn.Close()

	if err := s.writeCommand("RCV"); err != nil {
		return err
	}

	var (
		blockCount int64
		totalStart = time.Now()
	)

	for {
		start := time.Now()

		_, err := io.Copy(s.conn, dataReader)
		if err != nil {
			// If we get any of these errors, it probably just means that the server closed the connection
			if err == io.EOF || err == io.ErrClosedPipe || err == syscall.EPIPE {
				break
			}
			if operr, ok := err.(*net.OpError); ok {
				log.Printf("%s", operr.Err)
			}

			if operr, ok := err.(*net.OpError); ok && operr.Err.Error() == syscall.ECONNRESET.Error() {
				break
			}

			return fmt.Errorf("Error copying: %s", err)
		}

		bps := float64(1024*blockSize*8) / (float64(time.Since(start).Nanoseconds()) / float64(time.Second.Nanoseconds()))
		if bps < t.Send.Min {
			t.Send.Min = bps
		}
		if bps > t.Send.Max {
			t.Send.Max = bps
		}
		blockCount++

		dataReader.Seek(0, 0)

		if time.Since(totalStart) > time.Duration(throughputTestLength)*time.Second {
			break
		}
	}

	// average bit per second
	t.Send.Avg = float64(1024*blockSize*blockCount*8) / (float64(time.Since(totalStart).Nanoseconds()) / float64(time.Second.Nanoseconds()))

	return nil
}

func (s *sparkClient) runRecvTest(t *testResult) error {
	if err := s.connect(); err != nil {
		return err
	}
	defer s.conn.Close()

	if err := s.writeCommand("SND"); err != nil {
		return err
	}

	var (
		blockCount int64
		totalStart = time.Now()
	)

	for {
		start := time.Now()

		_, err := io.CopyN(ioutil.Discard, s.conn, 1024*blockSize)
		if err != nil {
			// If we get any of these errors, it probably just means that the server closed the connection
			if err == io.EOF || err == io.ErrClosedPipe || err == syscall.EPIPE {
				break
			}

			if operr, ok := err.(*net.OpError); ok && operr.Err.Error() == syscall.ECONNRESET.Error() {
				break
			}

			return fmt.Errorf("Error copying: %s", err)
		}

		bps := float64(1024*blockSize*8) / (float64(time.Since(start).Nanoseconds()) / float64(time.Second.Nanoseconds()))
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
	t.Receive.Avg = float64(1024*blockSize*blockCount*8) / (float64(time.Since(totalStart).Nanoseconds()) / float64(time.Second.Nanoseconds()))

	return nil
}
