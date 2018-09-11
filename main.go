package main

import (
	"log"
	"os"
	"os/signal"
	"time"

	// Caddy
	plugin "github.com/lucaslorentz/caddy-docker-proxy/plugin"
	"github.com/mholt/caddy"
	_ "github.com/mholt/caddy/caddyhttp"
	httpserver "github.com/mholt/caddy/caddyhttp/httpserver"

	// Plugins
	_ "github.com/caddyserver/dnsproviders/route53"
)

var instance *caddy.Instance
var loader *plugin.DockerLoader

func main() {
	httpserver.GracefulTimeout = 20 * time.Second

	caddy.AppName = "Caddy Docker Proxy"
	caddy.AppVersion = "0.1.4"

	loader = plugin.CreateDockerLoader(reload)

	input, err := loader.Load("http")
	if err != nil {
		log.Fatal(err)
	}

	instance, err = caddy.Start(input)
	if err != nil {
		log.Fatal(err)
	}

	trapShutdown()

	select {}
}

func reload() {
	log.Printf("[INFO] Reloading\n")

	instance.ShutdownCallbacks()

	err := instance.Stop()
	if err != nil {
		log.Printf("[ERROR] %v", err.Error())
		log.Fatal(err)
	}

	input, err := loader.Load("http")
	if err != nil {
		log.Printf("[ERROR] %v", err.Error())
		log.Fatal(err)
	}

	instance, err = caddy.Start(input)
	if err != nil {
		log.Printf("[ERROR] %v", err.Error())
		log.Fatal(err)
	}
}

func trapShutdown() {
	go func() {
		shutdown := make(chan os.Signal, 1)
		signal.Notify(shutdown, os.Interrupt)

		for i := 0; true; i++ {
			<-shutdown

			if i > 0 {
				log.Println("[INFO] SIGINT: Force quit")
				os.Exit(2)
			}

			log.Println("[INFO] SIGINT: Shutting down")

			go func() {
				exitCode := 0

				if errs := instance.ShutdownCallbacks(); len(errs) > 0 {
					for _, err := range errs {
						log.Printf("[ERROR] shutdown: %v", err)
					}
					exitCode = 1
				}

				if stopErr := instance.Stop(); stopErr != nil {
					exitCode = 1
				}

				os.Exit(exitCode)
			}()
		}
	}()
}
