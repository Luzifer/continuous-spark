package main

import (
	"bufio"
	"fmt"
	"math"
	"net"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	protocolVersion      = 0x00             // Protocol Version
	blockSize            = 200              // size (KB) of each block of data copied to/from remote
	throughputTestLength = 10 * time.Second // length of time to conduct each throughput test
	numPings             = 30               // number of pings to attempt

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
	bindInterfaceName string
	remote            string
	conn              net.Conn
	reader            *bufio.Reader
}

func newSparkClient(hostname string, port int, bindInterfaceName string) *sparkClient {
	return &sparkClient{
		bindInterfaceName: bindInterfaceName,
		remote:            fmt.Sprintf("%s:%d", hostname, port),
	}
}

func (s *sparkClient) dial() error {
	d := net.Dialer{}

	if s.bindInterfaceName != "" {
		iface, err := net.InterfaceByName(s.bindInterfaceName)
		if err != nil {
			return fmt.Errorf("selecting interface: %w", err)
		}

		addrs, err := iface.Addrs()
		if err != nil {
			return fmt.Errorf("getting interface IPs: %w", err)
		}

		if len(addrs) == 0 {
			return fmt.Errorf("no addresses found on interface")
		}

		d.LocalAddr = &net.TCPAddr{IP: addrs[0].(*net.IPNet).IP}
		logrus.WithField("ip", d.LocalAddr).Warn("Set local address")
	}

	c, err := d.Dial("tcp", s.remote)
	if err != nil {
		return fmt.Errorf("dialing remote server: %w", err)
	}
	s.conn = c

	return nil
}

func (s *sparkClient) connect() (err error) {
	if err = s.dial(); err != nil {
		return fmt.Errorf("connecting to remote %q: %w", s.remote, err)
	}

	s.reader = bufio.NewReader(s.conn)

	if err = s.writeCommand(fmt.Sprintf("HELO%d", protocolVersion)); err != nil {
		return fmt.Errorf("writing HELO command: %w", err)
	}

	return s.readGreeting()
}

func (s *sparkClient) writeCommand(command string) (err error) {
	if _, err = fmt.Fprintf(s.conn, "%s\r\n", command); err != nil {
		return fmt.Errorf("sending command %q: %w", command, err)
	}
	return nil
}

func (s *sparkClient) readGreeting() error {
	if helo, err := s.reader.ReadString('\n'); err != nil || strings.TrimSpace(helo) != "HELO" {
		return fmt.Errorf("unexpected response to greeting")
	}

	cn, err := s.reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading cn string: %w", err)
	}

	cn = strings.TrimSpace(cn)
	if cn == "none" {
		cn = s.remote
	}

	loc, err := s.reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading loc string: %w", err)
	}

	loc = strings.TrimSpace(loc)

	logrus.WithFields(logrus.Fields{
		"cn":       cn,
		"location": loc,
	}).Debug("Connected to server")

	return nil
}
