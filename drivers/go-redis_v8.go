package drivers

import (
	"context"

	"github.com/go-redis/redis/v8"
)

// GoRedisV8ClusterClient is a driver for GoRedisV8.
type GoRedisV8ClusterClient struct {
	r *redis.ClusterClient
}

// NewGoRedisV8ClusterClient returns a new GoRedisV8ClusterClient.
func NewGoRedisV8ClusterClient(c *redis.ClusterClient) *GoRedisV8ClusterClient {
	return &GoRedisV8ClusterClient{r: c}
}

// Eval a script on a single key (single key supported in order to respect redis cluster compatibility)
func (c *GoRedisV8ClusterClient) Eval(ctx context.Context, script, key string, args []any) (any, error) {
	return c.r.Eval(ctx, script, []string{key}, args...).Int64()
}
