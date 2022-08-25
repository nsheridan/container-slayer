
container-slayer is a tool for restarting unhealthy Docker containers.

## Installation & Usage

Docker container

Important: you need to mount the Docker socket inside the running container.
```
docker run --name slayer -v /var/run/docker.sock:/var/run/docker.sock --restart always --detach nsheridan/container-slayer
```

Install from source
```
go install nsheridan.dev/container-slayer@latest
```

### Configuration

Configuration is controlled by flags:
```
% container-slayer -h
Usage of container-slayer:
  -filter string
    	Only restart containers with this label. 'all' means all containers will be considered. (default "all")
  -interval duration
    	Interval between checks (default 1m1s)
  -socket string
    	Path to Docker socket (default "/var/run/docker.sock")
  -timeout duration
    	Time to wait for Docker to respond (default 30s)
  -unhealthy_count int
    	Number of consecutive unhealthy probes before restarting the container (default 3)
```

### Usage

container-slayer only operates on containers which have a [HEALTHCHECK](https://docs.docker.com/engine/reference/builder/#healthcheck) set in the image.


#### labels

By default container-slayer will consider any unhealthy container for restart. Add labels to your containers to restrict slayer's operations to a set of containers:

```
docker run -p 80:80 --name httpbin --label safe-to-reap kennethreitz/httpbin
```

Then use the `-filter` flag to limit restarts to just those labeled containers.
```
container-slayer -filter safe-to-reap
```
