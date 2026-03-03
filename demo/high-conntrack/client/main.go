package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	target := flag.String("target", "127.0.0.1:18080", "Server address")
	total := flag.Int("total", 10000, "Total connection attempts")
	concurrency := flag.Int("concurrency", 200, "Concurrent workers")
	timeout := flag.Duration("timeout", 2*time.Second, "Dial/read/write timeout")
	hold := flag.Duration("hold", 0, "Keep connection open before close")
	readReply := flag.Bool("read-reply", false, "Read one response from server")
	payload := flag.String("payload", "ping\n", "Payload to write")
	keepAlive := flag.Duration("keepalive", 30*time.Second, "TCP keepalive period")
	progressEvery := flag.Int("progress-every", 1000, "Print progress every N attempts")
	flag.Parse()

	if *total <= 0 || *concurrency <= 0 {
		log.Fatal("total and concurrency must be > 0")
	}

	start := time.Now()

	var success atomic.Int64
	var timeoutErr atomic.Int64
	var refusedErr atomic.Int64
	var resetErr atomic.Int64
	var otherErr atomic.Int64
	var done atomic.Int64

	jobs := make(chan struct{}, *concurrency)
	var wg sync.WaitGroup

	dialer := &net.Dialer{
		Timeout:   *timeout,
		KeepAlive: *keepAlive,
	}

	worker := func() {
		defer wg.Done()
		for range jobs {
			err := runOnce(dialer, *target, *timeout, *hold, *payload, *readReply)
			if err == nil {
				success.Add(1)
			} else {
				switch classify(err) {
				case "timeout":
					timeoutErr.Add(1)
				case "refused":
					refusedErr.Add(1)
				case "reset":
					resetErr.Add(1)
				default:
					otherErr.Add(1)
				}
			}

			currentDone := done.Add(1)
			if *progressEvery > 0 && int(currentDone)%*progressEvery == 0 {
				log.Printf("progress done=%d/%d success=%d fail=%d", currentDone, *total, success.Load(), currentDone-success.Load())
			}
		}
	}

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go worker()
	}

	for i := 0; i < *total; i++ {
		jobs <- struct{}{}
	}
	close(jobs)
	wg.Wait()

	elapsed := time.Since(start)
	fail := int64(*total) - success.Load()
	rate := float64(*total) / elapsed.Seconds()

	fmt.Printf("target=%s total=%d concurrency=%d elapsed=%s rate=%.0f conn/s\n", *target, *total, *concurrency, elapsed.Round(time.Millisecond), rate)
	fmt.Printf("success=%d fail=%d timeout=%d refused=%d reset=%d other=%d\n", success.Load(), fail, timeoutErr.Load(), refusedErr.Load(), resetErr.Load(), otherErr.Load())
}

func runOnce(dialer *net.Dialer, target string, timeout, hold time.Duration, payload string, readReply bool) error {
	conn, err := dialer.Dial("tcp", target)
	if err != nil {
		return err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))

	if payload != "" {
		if _, err := io.WriteString(conn, payload); err != nil {
			return err
		}
	}

	if hold > 0 {
		time.Sleep(hold)
	}

	if readReply {
		buf := make([]byte, 64)
		_, err := conn.Read(buf)
		if err != nil && err != io.EOF {
			return err
		}
	}
	return nil
}

func classify(err error) string {
	if err == nil {
		return ""
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "timeout"
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "refused"
	}
	if errors.Is(err, syscall.ECONNRESET) {
		return "reset"
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return "timeout"
	}
	return "other"
}

func init() {
	flag.Usage = func() {
		fmt.Println("TCP burst client for timeout/TIME_WAIT/SNAT lab")
		flag.PrintDefaults()
	}
}
