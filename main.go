package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"flag"
)

func main() {
	certFile := flag.String("cert", "", "certificate `path` for TLS")
	keyFile := flag.String("key", "", "key `path` for TLS")
	recordsFile := flag.String("records", "", "`path` to store records")
	flag.Parse()
	addr := flag.Arg(0)
	if addr == "" {
		addr = ":http"
	}

	server := &Server{
		addr: addr,
		certFile: *certFile,
		keyFile: *keyFile,
		recordsFile: *recordsFile,
	}
	err := server.Start()
	if err != nil {
		log.Printf("Failed to start: %v", err)
		return
	}

	log.Println("Running. Send signal to exit...")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
	log.Println("Exiting...")

	server.Stop()
}
