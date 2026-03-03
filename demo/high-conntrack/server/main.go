package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"sync/atomic"
	"time"
)

func main() {
	listenAddr := flag.String("listen", ":18080", "TCP listen address")
	readFirst := flag.Bool("read-first", true, "Read once from client before responding")
	readTimeout := flag.Duration("read-timeout", 2*time.Second, "Read timeout per connection")
	hold := flag.Duration("hold", 0, "How long to hold a connection before close")
	reply := flag.String("reply", "ok\n", "Reply payload")
	writeDelay := flag.Duration("write-delay", 0, "Delay before writing reply")
	flag.Parse()

	ln, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		log.Fatalf("listen error: %v", err)
	}
	defer ln.Close()

	log.Printf("server listening on %s", *listenAddr)

	var totalAccepted atomic.Int64
	var active atomic.Int64

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			log.Printf("stats accepted=%d active=%d", totalAccepted.Load(), active.Load())
		}
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}

		totalAccepted.Add(1)
		active.Add(1)
		go func(c net.Conn) {
			defer c.Close()
			defer active.Add(-1)

			if *readFirst {
				_ = c.SetReadDeadline(time.Now().Add(*readTimeout))
				buf := make([]byte, 1024)
				_, err := c.Read(buf)
				if err != nil && err != io.EOF {
					return
				}
			}

			if *hold > 0 {
				time.Sleep(*hold)
			}
			if *writeDelay > 0 {
				time.Sleep(*writeDelay)
			}
			if *reply != "" {
				_, _ = io.WriteString(c, *reply)
			}
		}(conn)
	}
}

func init() {
	flag.Usage = func() {
		fmt.Println("Simple TCP server for timeout/TIME_WAIT/SNAT lab")
		flag.PrintDefaults()
	}
}
