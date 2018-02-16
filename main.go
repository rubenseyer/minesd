package main

import (
	"flag"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	certFile := flag.String("cert", "", "certificate `path` for TLS")
	keyFile := flag.String("key", "", "key `path` for TLS")
	recordsFile := flag.String("records", "", "`path` to store records")
	logFile := flag.String("log", "", "`path` to log file")
	flag.Parse()
	addr := flag.Arg(0)
	if addr == "" {
		addr = ":http"
	}

	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0640)
		if err != nil {
			log.Fatalf("error opening file: %v", err)
		}
		defer f.Close()
		mw := io.MultiWriter(os.Stdout, f)
		log.SetOutput(mw)
	}

	server := &Server{
		addr:        addr,
		certFile:    *certFile,
		keyFile:     *keyFile,
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
