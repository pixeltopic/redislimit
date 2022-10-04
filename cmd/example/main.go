package main

import (
	"context"
	"fmt"
	"os"
	"redislimit"
	"redislimit/drivers"
	"time"

	"github.com/go-redis/redis/v8"
)

func main() {
	addr, ok := os.LookupEnv("REDIS_ENDPOINT")
	if !ok {
		panic(any("REDIS_ENDPOINT must be set"))
	}

	client := redislimit.New(
		drivers.NewGoRedisV8ClusterClient(redis.NewClusterClient(&redis.ClusterOptions{Addrs: []string{addr}, PoolSize: 5})),
		2, redislimit.WithWindowSize(time.Minute))

	fmt.Println(client.Allow(context.Background(), "foo"))
	fmt.Println(client.Allow(context.Background(), "foo"))
	fmt.Println(client.Allow(context.Background(), "foo"))
}
