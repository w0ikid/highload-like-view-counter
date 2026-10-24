package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	goredis "github.com/redis/go-redis/v9"
	"time"
)

var rateScript = goredis.NewScript(`
local count=redis.call('INCR',KEYS[1])
if count==1 then redis.call('PEXPIRE',KEYS[1],ARGV[1]) end
return count
`)

type RateLimiter struct {
	Client  *goredis.Client
	Timeout time.Duration
}

func (l *RateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, l.Timeout)
	defer cancel()
	sum := sha256.Sum256([]byte(key))
	count, err := rateScript.Run(ctx, l.Client, []string{"auth:rate:" + hex.EncodeToString(sum[:])}, 60000).Int64()
	return count <= 30, err
}
