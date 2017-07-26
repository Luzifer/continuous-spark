package main

import (
	"bufio"
	"fmt"
	"math"
	"net"
	"strings"

	log "github.com/sirupsen/logrus"
)

const (
	protocolVersion      uint16 = 0x00 // Protocol Version
	blockSize            int64  = 200  // size (KB) of each block of data copied to/from remote
	throughputTestLength uint   = 10   // length of time to conduct each throughput test
	maxPingTestLength    uint   = 10   // maximum time for ping test to complete
	numPings             int    = 30   // number of pings to attempt

	KBPS = 1024.0
	MBPS = 1024.0 * KBPS
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
		t.Receive.Min/MBPS,
		t.Receive.Max/MBPS,
		t.Receive.Avg/MBPS,
		t.Send.Min/MBPS,
		t.Send.Max/MBPS,
		t.Send.Avg/MBPS,
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
		return err
	}
	s.conn = c

	return nil
}

func (s *sparkClient) connect() error {
	if err := s.dial(); err != nil {
		return fmt.Errorf("Unable to connect to sparkyfish-server %q: %s", s.remote, err)
	}

	s.reader = bufio.NewReader(s.conn)

	if err := s.writeCommand(fmt.Sprintf("HELO%d", protocolVersion)); err != nil {
		return err
	}

	return s.readGreeting()
}

func (s *sparkClient) writeCommand(command string) error {
	if _, err := fmt.Fprintf(s.conn, "%s\r\n", command); err != nil {
		return fmt.Errorf("Unable to send command %q: %s", command, err)
	}
	return nil
}

func (s *sparkClient) readGreeting() error {
	if helo, err := s.reader.ReadString('\n'); err != nil || strings.TrimSpace(helo) != "HELO" {
		return fmt.Errorf("Unexpected response to greeting")
	}

	cn, err := s.reader.ReadString('\n')
	if err != nil {
		return err
	}

	cn = strings.TrimSpace(cn)
	if cn == "none" {
		cn = s.remote
	}

	loc, err := s.reader.ReadString('\n')
	if err != nil {
		return err
	}

	loc = strings.TrimSpace(loc)

	log.Debugf("Connected to %q in location %q", cn, loc)
	return nil
}
