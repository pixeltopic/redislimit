package redislimit

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Driver implements the Redis Driver.
type Driver interface {
	Eval(ctx context.Context, script, key string, args []any) (any, error)
}

// Config of Client.
type Config struct {
	BucketPrecision   time.Duration // Bucket sizes (defaults to 1 minute, so 60 per hour)
	WindowSize        time.Duration // Window to look back on (defaults to 1 minute)
	ThresholdInWindow int64
	StaleBucketAge    time.Duration // absolute oldest age of a bucket before it gets pruned. useful on on-off calls for other precisions
}

// Client for rate limiting.
type Client struct {
	conf Config
	d    Driver
}

// Allow a key through or deny it
func (c *Client) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now()
	startOfWindow := now.Add(-1 * c.conf.WindowSize).Truncate(c.conf.BucketPrecision).Unix()
	endOfWindow := now.Truncate(c.conf.BucketPrecision).Unix()

	res, err := c.d.Eval(ctx, handleRateScript, key, []any{
		now, startOfWindow, endOfWindow, c.conf.BucketPrecision.Seconds(), c.conf.StaleBucketAge.Seconds()})
	if err != nil {
		return false, err
	}
	tokens, ok := res.(int64)
	if tokens == -1 {
		return false, fmt.Errorf("window size is less than or equal to 0 seconds: %v seconds", endOfWindow-startOfWindow)
	}
	if tokens == -2 {
		return false, errors.New("invalid arguments provided to rate limit script")
	}
	if !ok {
		return false, fmt.Errorf("could not convert %T to int64", res)
	}
	return tokens <= c.conf.ThresholdInWindow, nil
}
