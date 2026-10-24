// Package redis owns the project's Redis connection; no private cache library is used.
package redis

import (
	goredis "github.com/redis/go-redis/v9"
	"time"
)

func New(addr string, timeout time.Duration) *goredis.Client {
	return goredis.NewClient(&goredis.Options{
		Addr: addr, MaxRetries: -1, ContextTimeoutEnabled: true,
		DialTimeout: timeout, ReadTimeout: timeout, WriteTimeout: timeout,
	})
}
