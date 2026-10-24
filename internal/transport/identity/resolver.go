package identity

import (
	"context"
	"highload-counters/internal/domain"
	"strconv"
	"strings"
)

type Auth interface {
	Me(context.Context, string) (domain.User, error)
}
type Resolver struct {
	Mode string
	Auth Auth
}

func (r Resolver) Resolve(ctx context.Context, testID, authorization string) (int64, error) {
	if r.Mode == "test" {
		id, err := strconv.ParseInt(testID, 10, 64)
		if err != nil || id <= 0 {
			return 0, domain.ErrUnauthorized
		}
		return id, nil
	}
	fields := strings.Fields(authorization)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
		return 0, domain.ErrUnauthorized
	}
	user, err := r.Auth.Me(ctx, fields[1])
	return user.ID, err
}
