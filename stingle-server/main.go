// The stingle-server binary is an API server that's compatible with the
// Stingle Photos app (https://github.com/stingle/stingle-photos-android)
// published by stingle.org.
//
// For the app to connect to this server, it has to the recompiled with
// api_server_url set to point to this server.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"stingle-server/database"
	"stingle-server/log"
	"stingle-server/server"
)

var (
	dbFlag   = flag.String("db", "", "The directory name of the database.")
	address  = flag.String("address", "127.0.0.1:8080", "The local address to use.")
	baseURL  = flag.String("base-url", "", "The base URL of the generated download links. If empty, the links will generated using the Host headers of the incoming requests, i.e. https://HOST/.")
	certFile = flag.String("cert", "", "The name of the file containing the TLS cert to use. If neither -cert or -key is set, the server will not use TLS.")
	keyFile  = flag.String("key", "", "The name of the file containing the TLS private key to use.")
	logLevel = flag.Int("v", 3, "The level of logging verbosity: 1:Error 2:Info 3:Debug")
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [flags]\n\nFlags:\n", os.Args[0])
	flag.PrintDefaults()
	os.Exit(64)
}

func main() {
	rand.Seed(int64(time.Now().Nanosecond()))
	flag.Usage = usage
	flag.Parse()
	log.Level = *logLevel

	if *dbFlag == "" {
		log.Error("--db must be set")
		usage()
	}
	if *address == "" {
		log.Error("--address must be set")
		usage()
	}
	if (*certFile == "") != (*keyFile == "") {
		log.Error("--cert and --key must either both be set or unset.")
		usage()
	}
	db := database.New(*dbFlag)
	s := server.New(db, *address)
	s.BaseURL = *baseURL

	done := make(chan struct{})
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT)
		signal.Notify(ch, syscall.SIGTERM)
		sig := <-ch
		log.Infof("Received signal %d (%s)", sig, sig)
		if err := s.Shutdown(); err != nil {
			log.Errorf("s.Shutdown: %v", err)
		}
		close(done)
	}()

	log.Info("Starting server")
	if *certFile == "" {
		if err := s.Run(); err != http.ErrServerClosed {
			log.Errorf("s.Run: %v", err)
		}
	} else {
		if err := s.RunWithTLS(*certFile, *keyFile); err != http.ErrServerClosed {
			log.Errorf("s.RunWithTLS: %v", err)
		}
	}
	<-done
	log.Info("Server exited cleanly.")
}
