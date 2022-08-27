package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	docker "github.com/docker/docker/client"
	"golang.org/x/sync/errgroup"
)

var (
	interval       time.Duration
	timeout        time.Duration
	unhealthyCount int
	sockPath       string
	filterLabel    string

	dockerAPIVersion = "1.38"
)

func init() {
	flag.DurationVar(&interval, "interval", 61*time.Second, "Interval between checks")
	flag.DurationVar(&timeout, "timeout", 30*time.Second, "Time to wait for Docker to respond")
	flag.IntVar(&unhealthyCount, "unhealthy_count", 3, "Number of consecutive unhealthy probes before restarting the container")
	flag.StringVar(&sockPath, "socket", "/var/run/docker.sock", "Path to Docker socket")
	flag.StringVar(&filterLabel, "filter", "all", "Only restart containers with this label. 'all' means all containers will be considered.")
}

func dockerClient(timeout time.Duration, sockPath string) (*docker.Client, error) {
	d := new(net.Dialer)
	hClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			IdleConnTimeout:       2 * time.Minute,
			ResponseHeaderTimeout: timeout,
			DialContext: func(ctx context.Context, net, addr string) (net.Conn, error) {
				return d.DialContext(ctx, "unix", sockPath)
			},
		},
	}
	return docker.NewClientWithOpts(docker.WithHTTPClient(hClient), docker.WithVersion(dockerAPIVersion))
}

func main() {
	flag.Parse()

	containers := make(chan types.Container)
	ctx := context.Background()
	g, ctx := errgroup.WithContext(ctx)

	client, err := dockerClient(timeout, sockPath)
	if err != nil {
		log.Fatalf("error creating docker client: %v\n", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	g.Go(func() error {
		for {
			err := getUnhealthy(ctx, client, containers)
			if err != nil {
				log.Printf("error fetching containers: %v\n", err)
			}
			select {
			case <-ticker.C:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	})
	go func() {
		err := g.Wait()
		log.Fatalf("fetch loop exited: %v\n", err)
	}()

	unhealthy := map[string]int{}
	for c := range containers {
		unhealthy[c.ID]++
		if unhealthy[c.ID] >= unhealthyCount {
			log.Printf("Restarting container %s [%s]", c.ID, c.Names[0])
			if err := client.ContainerRestart(ctx, c.ID, &timeout); err != nil {
				log.Printf("Error restarting container: %v", err)
			}
			delete(unhealthy, c.ID)
		}
	}
}

func getUnhealthy(ctx context.Context, client *docker.Client, ch chan<- types.Container) error {
	f := filters.NewArgs(filters.Arg("health", "unhealthy"))
	if filterLabel != "all" {
		f.Add("label", filterLabel)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	unhealthy, err := client.ContainerList(ctx, types.ContainerListOptions{
		Filters: f,
	})
	if err != nil {
		return err
	}
	for _, c := range unhealthy {
		ch <- c
	}
	return nil
}
