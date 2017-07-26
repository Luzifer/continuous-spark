package main

import (
	"math"
	"sort"
	"time"
)

type pingHistory []int64

func (s *sparkClient) ExecutePingTest(t *testResult) error {
	ph := pingHistory{}

	if err := s.connect(); err != nil {
		return err
	}

	if err := s.writeCommand("ECO"); err != nil {
		return err
	}

	buf := make([]byte, 1)

	for i := 0; i < numPings; i++ {
		start := time.Now()
		s.conn.Write([]byte{46})

		if _, err := s.conn.Read(buf); err != nil {
			return err
		}

		ph = append(ph, time.Since(start).Nanoseconds()/1000)
	}

	ph = ph.toMilli()

	t.Ping.Min, t.Ping.Max = ph.minMax()
	t.Ping.Avg = ph.mean()
	t.Ping.Dev = ph.stdDev()

	return nil
}

// toMilli Converts our ping history to milliseconds for display purposes
func (h *pingHistory) toMilli() []int64 {
	var pingMilli []int64

	for _, v := range *h {
		pingMilli = append(pingMilli, v/1000)
	}

	return pingMilli
}

// mean generates a statistical mean of our historical ping times
func (h *pingHistory) mean() float64 {
	var sum uint64
	for _, t := range *h {
		sum = sum + uint64(t)
	}

	return float64(sum / uint64(len(*h)))
}

// variance calculates the variance of our historical ping times
func (h *pingHistory) variance() float64 {
	var sqDevSum float64

	mean := h.mean()

	for _, t := range *h {
		sqDevSum = sqDevSum + math.Pow((float64(t)-mean), 2)
	}
	return sqDevSum / float64(len(*h))
}

// stdDev calculates the standard deviation of our historical ping times
func (h *pingHistory) stdDev() float64 {
	return math.Sqrt(h.variance())
}

func (h *pingHistory) minMax() (float64, float64) {
	var hist []int
	for _, v := range *h {
		hist = append(hist, int(v))
	}
	sort.Ints(hist)
	return float64(hist[0]), float64(hist[len(hist)-1])
}
