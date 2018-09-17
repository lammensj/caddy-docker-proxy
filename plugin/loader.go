package plugin

import (
	"bytes"
	"context"
	"log"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/mholt/caddy"
)

const poolInterval = 10 * time.Second

// DockerLoader generates caddy files from docker swarm information
type DockerLoader struct {
	reload       func()
	initialized  bool
	dockerClient *client.Client
	generator    *CaddyfileGenerator
	timer        *time.Timer
	skipEvents   bool
	input        caddy.CaddyfileInput
}

// CreateDockerLoader creates a docker loader
func CreateDockerLoader(reload func()) *DockerLoader {
	return &DockerLoader{
		reload: reload,
		input: caddy.CaddyfileInput{
			ServerTypeName: "http",
		},
	}
}

// Load returns the current caddy file input
func (dockerLoader *DockerLoader) Load(serverType string) (caddy.Input, error) {
	if serverType != "http" {
		return nil, nil
	}
	if !dockerLoader.initialized {
		dockerLoader.initialized = true

		dockerClient, err := client.NewEnvClient()
		if err != nil {
			log.Printf("Docker connection failed: %v", err)
			return nil, nil
		}

		dockerPing, err := dockerClient.Ping(context.Background())
		if err != nil {
			log.Printf("Docker ping failed: %v", err)
			return nil, nil
		}

		dockerClient.NegotiateAPIVersionPing(dockerPing)

		dockerLoader.dockerClient = dockerClient
		dockerLoader.generator = CreateGenerator(
			WrapDockerClient(dockerClient),
			CreateDockerUtils(),
			GetGeneratorOptions(),
		)

		dockerLoader.timer = time.AfterFunc(poolInterval, func() {
			dockerLoader.update(true)
		})

		dockerLoader.update(false)

		go dockerLoader.monitorEvents()
	}
	return dockerLoader.input, nil
}

func (dockerLoader *DockerLoader) monitorEvents() {
	args := filters.NewArgs()
	args.Add("scope", "swarm")
	args.Add("scope", "local")
	args.Add("type", "service")
	args.Add("type", "container")
	args.Add("type", "config")

	eventsChan, errorChan := dockerLoader.dockerClient.Events(context.Background(), types.EventsOptions{
		Filters: args,
	})

	for {
		select {
		case event := <-eventsChan:
			if dockerLoader.skipEvents {
				continue
			}

			update := (event.Type == "container" && event.Action == "start") ||
				(event.Type == "container" && event.Action == "stop") ||
				(event.Type == "service" && event.Action == "create") ||
				(event.Type == "service" && event.Action == "update") ||
				(event.Type == "service" && event.Action == "remove") ||
				(event.Type == "config" && event.Action == "create") ||
				(event.Type == "config" && event.Action == "remove")

			if update {
				dockerLoader.skipEvents = true
				dockerLoader.timer.Reset(100 * time.Millisecond)
			}
		case err := <-errorChan:
			log.Println(err)
		}
	}
}

func (dockerLoader *DockerLoader) update(reloadIfChanged bool) bool {
	dockerLoader.timer.Reset(poolInterval)
	dockerLoader.skipEvents = false

	newContents := dockerLoader.generator.GenerateCaddyFile()

	if bytes.Equal(dockerLoader.input.Contents, newContents) {
		return false
	}

	newInput := caddy.CaddyfileInput{
		ServerTypeName: "http",
		Contents:       newContents,
	}

	if err := caddy.ValidateAndExecuteDirectives(newInput, nil, true); err != nil {
		log.Printf("[ERROR] CaddyFile error: %s", err)
		log.Printf("[INFO] Wrong CaddyFile:\n%s", newContents)
	} else {
		log.Printf("[INFO] New CaddyFile:\n%s", newInput.Contents)

		dockerLoader.input = newInput

		if reloadIfChanged {
			dockerLoader.reload()
		}
	}

	return true
}
