package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/docker/docker/api/types/container"
	docker "github.com/docker/docker/client"
)

type HealthStatus = int64

const (
	HealthStatus_NONE      HealthStatus = 0
	HealthStatus_STARTING  HealthStatus = 1
	HealthStatus_HEALTHY   HealthStatus = 2
	HealthStatus_UNHEALTHY HealthStatus = 3
)

type Options struct {
	BaseUrl string
}

func parseOptions() *Options {
	options := &Options{}
	flag.StringVar(&options.BaseUrl, "base-url", os.Getenv("BASE_URL"), "URL prefix for HTTP server")
	flag.Parse()

	if options.BaseUrl != "" {
		options.BaseUrl = "/" + strings.Trim(options.BaseUrl, "/")
	}

	return options
}

func errorMiddleware(client *docker.Client, handler func(client *docker.Client, writer http.ResponseWriter) error) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		err := handler(client, writer)

		if err != nil {
			writer.WriteHeader(500)
			writer.Write([]byte(err.Error()))

		}
	}
}

func healthHandler(client *docker.Client, writer http.ResponseWriter) error {
	_, err := client.Ping(context.Background())

	if err != nil {
		return errors.New("nok")
	}

	writer.WriteHeader(200)
	writer.Write([]byte("ok"))
	return nil
}

func metricsHandler(client *docker.Client, writer http.ResponseWriter) error {
	containers, err := client.ContainerList(context.Background(), container.ListOptions{})

	if err != nil {
		slog.Error("failed to list containers", "err", err)
		return errors.New("internal server error")
	}

	output := "# HELP container_state_health_status Docker container Health checks status (mapped as int)\r\n"
	output += "# TYPE container_state_health_status gauge\r\n"

	for _, container := range containers {
		inspect, err := client.ContainerInspect(context.Background(), container.ID)

		if err != nil {
			slog.Error("failed to inspect container", "container_id", container.ID, "err", err)
			return errors.New("internal server error")
		}

		var health HealthStatus

		if inspect.State == nil || inspect.State.Health == nil {
			health = HealthStatus_NONE
		} else {
			switch inspect.State.Health.Status {
			case "starting":
				health = HealthStatus_STARTING
				break
			case "healthy":
				health = HealthStatus_HEALTHY
				break
			case "unhealthy":
				health = HealthStatus_UNHEALTHY
				break
			default:
				health = HealthStatus_NONE
				break
			}
		}

		output += fmt.Sprintf(
			"container_state_health_status{container_name=\"%s\", container_id=\"%s\"} %d\r\n",
			strings.TrimPrefix(inspect.Name, "/"),
			inspect.ID,
			health,
		)
	}

	writer.WriteHeader(200)
	writer.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(output)))
	writer.Write([]byte(output))
	return nil
}

func main() {
	options := parseOptions()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	client, err := docker.NewClientWithOpts(docker.FromEnv)

	if err != nil {
		logger.Error("failed to create docker client", "err", err)
		os.Exit(1)
	}

	http.HandleFunc(options.BaseUrl+"/metrics", errorMiddleware(client, metricsHandler))
	http.HandleFunc(options.BaseUrl+"/health", errorMiddleware(client, healthHandler))

	slog.Info("starting server", "base_url", options.BaseUrl)
	err = http.ListenAndServe("0.0.0.0:8080", nil)

	if err != nil {
		logger.Error("failed to listen on port 8080", "err", err)
		os.Exit(1)
	}
}
