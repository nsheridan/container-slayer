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

	dockerAPIVersion = "1.38"
)

func init() {
	flag.DurationVar(&interval, "interval", 61*time.Second, "Interval between checks")
	flag.DurationVar(&timeout, "timeout", 30*time.Second, "Time to wait for Docker to respond")
	flag.IntVar(&unhealthyCount, "unhealthy_count", 3, "Number of consecutive unhealthy probes before restarting the container")
	flag.StringVar(&sockPath, "socket", "/var/run/docker.sock", "Path to Docker socket")
}

func main() {
	flag.Parse()

	contsCh := make(chan types.Container)
	ctx := context.Background()
	g, ctx := errgroup.WithContext(ctx)

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

	client, err := docker.NewClientWithOpts(docker.WithHTTPClient(hClient), docker.WithVersion(dockerAPIVersion))
	if err != nil {
		log.Fatalf("error creating docker client: %v\n", err)
	}

	g.Go(func() error {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			ctx, cancel := context.WithDeadline(ctx, time.Now().Add(timeout))
			defer cancel()
			err := getUnhealthy(ctx, client, contsCh)
			if err != nil {
				log.Println(err)
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
		if err := g.Wait(); err != nil {
			log.Printf("Error retrieving containers: %v\n", err)
		}
	}()

	unhealthy := map[string]int{}
	for c := range contsCh {
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
	unhealthy, err := client.ContainerList(ctx, types.ContainerListOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key: "health", Value: "unhealthy",
		}),
	})
	if err != nil {
		return err
	}
	for _, c := range unhealthy {
		ch <- c
	}
	return nil
}
