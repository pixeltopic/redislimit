package redislimit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
)

func TestRateLimitScript(t *testing.T) {
	now := time.Unix(1664832874, 0)

	newBucket := func(bucketPosition int, precision time.Duration, count int) []any {
		return append([]any{}, fmt.Sprintf("%v:%v", now.Add(-1*time.Duration(bucketPosition)*precision).Truncate(precision).Unix(), precision.Seconds()), count)
	}

	const (
		allow = int64(1)
		deny  = int64(0)
	)

	// TODO: failed requests should not increment - confirm

	type testCase struct {
		name            string
		windowSize      time.Duration
		bucketPrecision time.Duration
		staleBucketAge  time.Duration
		threshold       int64
		testFunc        func(tc testCase, t *testing.T, server *miniredis.Miniredis, client *redis.ClusterClient, scriptArgs []any)
	}

	testCases := []testCase{
		{
			name:            "inserting a new key should succeed",
			windowSize:      time.Minute,
			bucketPrecision: time.Minute,
			staleBucketAge:  time.Hour,
			threshold:       2,
			testFunc: func(tc testCase, t *testing.T, server *miniredis.Miniredis, client *redis.ClusterClient, scriptArgs []any) {
				code, err := client.Eval(context.Background(), rateLimitScript, []string{"foo"}, scriptArgs...).Int64()
				if !assert.NoError(t, err) {
					return
				}
				assert.Equal(t, allow, code)
				assert.Greater(t, int64(server.TTL("foo")), int64(0))
			},
		},
		{
			name:            "inserting running a script on the key twice should return the proper token count",
			windowSize:      5 * time.Minute,
			bucketPrecision: time.Minute,
			staleBucketAge:  time.Hour,
			threshold:       2,
			testFunc: func(tc testCase, t *testing.T, server *miniredis.Miniredis, client *redis.ClusterClient, scriptArgs []any) {
				_, err := client.Eval(context.Background(), rateLimitScript, []string{"foo"}, scriptArgs...).Int64()
				if !assert.NoError(t, err) {
					return
				}
				code, err := client.Eval(context.Background(), rateLimitScript, []string{"foo"}, scriptArgs...).Int64()
				if !assert.NoError(t, err) {
					return
				}
				assert.Equal(t, allow, code)
				assert.Greater(t, int64(server.TTL("foo")), int64(0))
			},
		},
		{
			name:            "script should respect buckets based on window",
			windowSize:      5 * time.Minute,
			bucketPrecision: time.Minute,
			staleBucketAge:  time.Hour,
			threshold:       7,
			testFunc: func(tc testCase, t *testing.T, server *miniredis.Miniredis, client *redis.ClusterClient, scriptArgs []any) {

				// this first one is outside of the window, so it'll get pruned.
				_, err := client.HSet(context.Background(), "foo", newBucket(5, tc.bucketPrecision, 2)...).Result()
				if !assert.NoError(t, err) {
					return
				}
				// this one will get pruned because it is beyond stale bucket age despite it having another precision
				_, err = client.HSet(context.Background(), "foo", newBucket(5, time.Minute*15, 2)...).Result()
				if !assert.NoError(t, err) {
					return
				}
				_, err = client.HSet(context.Background(), "foo", newBucket(1, time.Minute*15, 2)...).Result()
				if !assert.NoError(t, err) {
					return
				}
				_, err = client.HSet(context.Background(), "foo", newBucket(4, tc.bucketPrecision, 2)...).Result()
				if !assert.NoError(t, err) {
					return
				}
				_, err = client.HSet(context.Background(), "foo", newBucket(1, tc.bucketPrecision, 2)...).Result()
				if !assert.NoError(t, err) {
					return
				}
				_, err = client.HSet(context.Background(), "foo", newBucket(0, tc.bucketPrecision, 2)...).Result()
				if !assert.NoError(t, err) {
					return
				}

				code, err := client.Eval(context.Background(), rateLimitScript, []string{"foo"}, scriptArgs...).Int64()
				if !assert.NoError(t, err) {
					return
				}

				assert.Equal(t, allow, code)
				assert.Greater(t, int64(server.TTL("foo")), int64(0))

				buckets, err := client.HGetAll(context.Background(), "foo").Result()
				if !assert.NoError(t, err) {
					return
				}
				assert.Len(t, buckets, 4)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockRedisServer, err := miniredis.Run()
			if err != nil {
				t.Fatal(err)
			}

			startOfWindow := now.Add(-1 * tc.windowSize).Truncate(tc.bucketPrecision).Unix()
			endOfWindow := now.Truncate(tc.bucketPrecision).Unix()

			c := redis.NewClusterClient(&redis.ClusterOptions{
				Addrs: []string{mockRedisServer.Addr()},
			})

			args := []interface{}{
				now.Unix(), startOfWindow, endOfWindow, tc.bucketPrecision.Seconds(), tc.staleBucketAge.Seconds(), tc.threshold}

			tc.testFunc(tc, t, mockRedisServer, c, args)
		})
	}
}
