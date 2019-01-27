package main

import (
	"bufio"
	"fmt"
	"math"
	"net"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	protocolVersion      uint16 = 0x00 // Protocol Version
	blockSize            int64  = 200  // size (KB) of each block of data copied to/from remote
	throughputTestLength uint   = 10   // length of time to conduct each throughput test
	numPings             int    = 30   // number of pings to attempt

	kbps = 1024.0
	mbps = 1024.0 * kbps
)

type testResult struct {
	Ping struct {
		Min, Max, Avg, Dev float64
	}
	Send struct {
		Min, Max, Avg float64
	}
	Receive struct {
		Min, Max, Avg float64
	}
}

func newTestResult() *testResult {
	r := &testResult{}
	r.Ping.Min = math.Inf(1)
	r.Ping.Max = math.Inf(-1)
	r.Send.Min = math.Inf(1)
	r.Send.Max = math.Inf(-1)
	r.Receive.Min = math.Inf(1)
	r.Receive.Max = math.Inf(-1)

	return r
}

func (t testResult) String() string {
	return fmt.Sprintf("Ping(ms): min=%.2f max=%.2f avg=%.2f stddev=%.2f | Download(Mbps): min=%.2f max=%.2f avg=%.2f | Upload(Mbps): min=%.2f max=%.2f avg=%.2f",
		t.Ping.Min,
		t.Ping.Max,
		t.Ping.Avg,
		t.Ping.Dev,
		t.Receive.Min/mbps,
		t.Receive.Max/mbps,
		t.Receive.Avg/mbps,
		t.Send.Min/mbps,
		t.Send.Max/mbps,
		t.Send.Avg/mbps,
	)
}

type sparkClient struct {
	remote string
	conn   net.Conn
	reader *bufio.Reader
}

func newSparkClient(hostname string, port int) *sparkClient {
	return &sparkClient{
		remote: fmt.Sprintf("%s:%d", hostname, port),
	}
}

func (s *sparkClient) dial() error {
	c, err := net.Dial("tcp", s.remote)
	if err != nil {
		return errors.Wrap(err, "Unable to dial")
	}
	s.conn = c

	return nil
}

func (s *sparkClient) connect() error {
	if err := s.dial(); err != nil {
		return errors.Wrapf(err, "Unable to connect to sparkyfish-server %q", s.remote)
	}

	s.reader = bufio.NewReader(s.conn)

	if err := s.writeCommand(fmt.Sprintf("HELO%d", protocolVersion)); err != nil {
		return errors.Wrap(err, "Unable to send HELO command")
	}

	return s.readGreeting()
}

func (s *sparkClient) writeCommand(command string) error {
	if _, err := fmt.Fprintf(s.conn, "%s\r\n", command); err != nil {
		return errors.Wrapf(err, "Unable to send command %q", command)
	}
	return nil
}

func (s *sparkClient) readGreeting() error {
	if helo, err := s.reader.ReadString('\n'); err != nil || strings.TrimSpace(helo) != "HELO" {
		return errors.New("Unexpected response to greeting")
	}

	cn, err := s.reader.ReadString('\n')
	if err != nil {
		return errors.Wrap(err, "Unable to read string")
	}

	cn = strings.TrimSpace(cn)
	if cn == "none" {
		cn = s.remote
	}

	loc, err := s.reader.ReadString('\n')
	if err != nil {
		return errors.Wrap(err, "Unable to read string")
	}

	loc = strings.TrimSpace(loc)

	log.WithFields(log.Fields{
		"cn":       cn,
		"location": loc,
	}).Debug("Connected to server")
	return nil
}
