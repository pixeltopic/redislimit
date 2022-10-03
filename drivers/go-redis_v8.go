package drivers

import (
	"context"

	"github.com/go-redis/redis/v8"
)

// GoRedisV8 driver for redislimit client
type GoRedisV8 struct {
	r *redis.Client
}

// Eval a script on a single key (single key supported in order to respect redis cluster compatibility)
func (d *GoRedisV8) Eval(ctx context.Context, script, key string, args []any) (any, error) {
	return d.r.Eval(ctx, script, []string{key}, args...).Result()
}
