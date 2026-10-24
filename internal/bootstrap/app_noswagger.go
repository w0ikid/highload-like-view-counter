//go:build !swagger

package bootstrap

import "net/http"

func registerSwagger(_ *http.ServeMux) {}
