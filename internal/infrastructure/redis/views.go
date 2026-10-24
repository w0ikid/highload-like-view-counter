package redis

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"highload-counters/internal/domain"
	"strconv"
	"time"
)

// One logical partition; every Lua key uses the same stable hash tag.
// The namespace also isolates integration tests and benchmark runs.
type Views struct {
	client *goredis.Client
	prefix string
	ttl    time.Duration
}

func NewViews(client *goredis.Client, namespace string, ttl time.Duration) *Views {
	return &Views{client, namespace + ":{views}:", ttl}
}

var recordViews = goredis.NewScript(`
local accepted=0
for i=2,#KEYS do
 if redis.call('SET',KEYS[i],'1','NX','PX',ARGV[1]) then
  redis.call('HINCRBY',KEYS[1],ARGV[i],1)
  accepted=accepted+1
 end
end
return {accepted,#KEYS-1-accepted}
`)

func (s *Views) Record(ctx context.Context, events []domain.ViewEvent) (domain.RecordViewsResult, error) {
	keys := []string{s.prefix + "pending"}
	args := []any{s.ttl.Milliseconds()}
	for _, event := range events {
		keys = append(keys, fmt.Sprintf("%sdedup:%d:%d", s.prefix, event.PostID, event.ViewerID))
		args = append(args, strconv.FormatInt(event.PostID, 10))
	}
	result, err := recordViews.Run(ctx, s.client, keys, args...).Int64Slice()
	if err != nil {
		return domain.RecordViewsResult{}, redisError(ctx, err)
	}
	return domain.RecordViewsResult{Accepted: int(result[0]), Duplicates: int(result[1])}, nil
}

var claimViews = goredis.NewScript(`
if redis.call('EXISTS',KEYS[2])==1 then return redis.call('HGETALL',KEYS[2]) end
if redis.call('EXISTS',KEYS[1])==0 then return {} end
redis.call('HSET',KEYS[1],'__batch',ARGV[1])
redis.call('RENAME',KEYS[1],KEYS[2])
return redis.call('HGETALL',KEYS[2])
`)

func (s *Views) Claim(ctx context.Context) (domain.FlushBatch, error) {
	values, err := claimViews.Run(ctx, s.client, []string{s.prefix + "pending", s.prefix + "inflight"}, uuid.NewString()).StringSlice()
	if err != nil {
		return domain.FlushBatch{}, redisError(ctx, err)
	}
	var batch domain.FlushBatch
	for i := 0; i < len(values); i += 2 {
		if values[i] == "__batch" {
			batch.ID = values[i+1]
			continue
		}
		id, e1 := strconv.ParseInt(values[i], 10, 64)
		delta, e2 := strconv.ParseInt(values[i+1], 10, 64)
		if e1 != nil || e2 != nil || id <= 0 || delta <= 0 {
			return domain.FlushBatch{}, fmt.Errorf("invalid in-flight batch")
		}
		batch.Deltas = append(batch.Deltas, domain.ViewDelta{PostID: id, Delta: delta})
	}
	if len(values) > 0 {
		if _, err := uuid.Parse(batch.ID); err != nil {
			return domain.FlushBatch{}, fmt.Errorf("invalid batch ID")
		}
	}
	return batch, nil
}

var ackViews = goredis.NewScript(`
if redis.call('HGET',KEYS[1],'__batch')==ARGV[1] then return redis.call('DEL',KEYS[1]) end
return 0
`)

func (s *Views) Ack(ctx context.Context, id string) error {
	_, err := ackViews.Run(ctx, s.client, []string{s.prefix + "inflight"}, id).Result()
	if err != nil {
		return redisError(ctx, err)
	}
	return nil
}
func redisError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return fmt.Errorf("%w: %w", domain.ErrUnavailable, err)
}
