// xdl-core is the Go backend of XDL, run as a child process of the macOS app.
// It exposes a bearer-authenticated JSON API on an ephemeral localhost port and
// writes {address, secret} to the -api-info file so the app can find it.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"xdl/backend"
)

const coreVersion = "1.0.0"

type apiInfo struct {
	Address string `json:"address"`
	Secret  string `json:"secret"`
	PID     int    `json:"pid"`
	Version string `json:"version"`
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[xdl-core] ")

	apiAddr := flag.String("api", "127.0.0.1:0", "listen address for the control API")
	apiInfoPath := flag.String("api-info", "", "path to write {address, secret} JSON for the app")
	apiSecret := flag.String("api-secret", "", "bearer secret (prefer XDL_API_SECRET env)")
	watchParent := flag.Bool("watch-parent", true, "exit when the parent process dies")
	flag.Parse()

	// Prefer the env var over argv (argv is visible to every local user in ps).
	secret := os.Getenv("XDL_API_SECRET")
	os.Unsetenv("XDL_API_SECRET")
	if secret == "" {
		secret = *apiSecret
	}
	if secret == "" {
		log.Fatal("refusing to run an unauthenticated control API: set XDL_API_SECRET")
	}

	if err := backend.InitDB(); err != nil {
		log.Printf("failed to initialize database: %v", err)
	}
	backend.KillAllExtractorProcesses()

	listener, err := net.Listen("tcp", *apiAddr)
	if err != nil {
		log.Fatalf("listen %s: %v", *apiAddr, err)
	}
	addr := listener.Addr().String()
	log.Printf("control API listening on %s", addr)

	if *apiInfoPath != "" {
		info := apiInfo{Address: addr, Secret: secret, PID: os.Getpid(), Version: coreVersion}
		data, _ := json.Marshal(info)
		if err := os.WriteFile(*apiInfoPath, data, 0o600); err != nil {
			log.Fatalf("write api-info %s: %v", *apiInfoPath, err)
		}
	}

	srv := newAPIServer(secret)
	shutdown := make(chan struct{})
	go srv.serve(listener, shutdown)

	// The app spawns us directly; if it dies, our parent becomes launchd (pid 1).
	if *watchParent {
		origPPID := os.Getppid()
		go func() {
			for {
				time.Sleep(2 * time.Second)
				if os.Getppid() != origPPID {
					log.Print("parent process gone, exiting")
					close(shutdown)
					return
				}
			}
		}()
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case s := <-sig:
		log.Printf("received %v, shutting down", s)
	case <-shutdown:
	}

	if *apiInfoPath != "" {
		os.Remove(*apiInfoPath)
	}
	backend.KillAllExtractorProcesses()
	backend.CloseDB()
	fmt.Fprintln(os.Stderr, "[xdl-core] bye")
}
