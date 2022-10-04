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

// Client for rate limiting which implements a Redis cluster based sliding window strategy.
//
// Some implementation quirks:
//
// A single key is associated with many buckets, classified by precision.
// If several rate limiters share a key with same precisions but use different window sizes, this may affect accuracy
// of results because buckets are expired based on window size.
type Client struct {
	conf config
	d    Driver
}

// config contains settings for sliding window rate limiting.
type config struct {
	BucketPrecision   time.Duration // Bucket sizes (defaults to 1 minute, so 60 per hour)
	WindowSize        time.Duration // Window to look back on (defaults to 1 minute)
	ThresholdInWindow int64
	StaleBucketAge    time.Duration // absolute oldest age of a bucket before it gets pruned. useful on on-off calls for other precisions
}

// ConfigOpt implements functional API options for Client.
type ConfigOpt interface {
	apply(conf *config)
}

type slidingWindowOptFunc func(conf *config)

func (f slidingWindowOptFunc) apply(conf *config) {
	f(conf)
}

// WithBucketPrecision updates the BucketPrecision of a rate limited key.
// BucketPrecision must represent a duration that divides 60 minutes evenly and must be at least 1 second.
// Invalid configs will default to 1 minute bucket precisions.
func WithBucketPrecision(precision time.Duration) ConfigOpt {
	return slidingWindowOptFunc(func(conf *config) {
		if precision >= time.Second && time.Hour%precision == 0 {
			conf.BucketPrecision = precision
		} else {
			conf.BucketPrecision = time.Minute
		}
	})
}

// WithWindowSize defines a window size to look back on for token counts.
func WithWindowSize(window time.Duration) ConfigOpt {
	return slidingWindowOptFunc(func(conf *config) {
		if window >= time.Second {
			conf.WindowSize = window
		} else {
			conf.WindowSize = time.Minute
		}
	})
}

// WithStaleBucketAge sets the oldest possible age of ANY bucket, regardless of precision.
// This is a catch-all for pruning stale buckets.
func WithStaleBucketAge(age time.Duration) ConfigOpt {
	return slidingWindowOptFunc(func(conf *config) {
		if age > 0 {
			conf.StaleBucketAge = age
		} else {
			conf.StaleBucketAge = time.Hour * 12
		}
	})
}

// New creates a new redis based rate limiter with a sliding window strategy.
func New(d Driver, tokenThreshold int64, opts ...ConfigOpt) *Client {
	conf := config{
		BucketPrecision:   time.Minute,
		WindowSize:        time.Minute,
		ThresholdInWindow: tokenThreshold,
		StaleBucketAge:    time.Hour,
	}

	for _, o := range opts {
		o.apply(&conf)
	}

	return &Client{
		conf: conf,
		d:    d,
	}
}

// Allow a key through or deny it.
func (c *Client) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now()
	startOfWindow := now.Add(-1 * c.conf.WindowSize).Truncate(c.conf.BucketPrecision).Unix()
	endOfWindow := now.Truncate(c.conf.BucketPrecision).Unix()

	res, err := c.d.Eval(ctx, rateLimitScript, key, []any{
		now.Unix(),
		startOfWindow,
		endOfWindow,
		c.conf.BucketPrecision.Seconds(),
		c.conf.StaleBucketAge.Seconds(),
		c.conf.ThresholdInWindow,
	})

	if err != nil {
		return false, err
	}
	result, ok := res.(int64)

	switch result {
	case -3:
		return false, errors.New("threshold must be an integer greater than 0")
	case -2:
		return false, errors.New("invalid arguments provided to rate limit script")
	case -1:
		return false, fmt.Errorf("window size is less than or equal to 0 seconds: %v seconds", endOfWindow-startOfWindow)
	}
	if !ok {
		return false, fmt.Errorf("could not convert %T to int64", res)
	}
	return result == 1, nil
}
