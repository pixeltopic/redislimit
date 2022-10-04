# redislimit

Redislimit is a simple rate limiter strategy that:

- implements a sliding window
- is compatible with redis clusters

This package is intended to be used when rate limiting must be 
distributed and persisted when a service (besides redis) is restarted.

## dependencies

- redis 6.X+
- go 1.19.1+

## installation

`go get github.com/pixeltopic/redislimit`

## running the example

`make build`
`docker compose up redislimit`