package proxy

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

const (
	tokenGuardProviderHeader = "X-TokenGuard-Provider"
	providerOpenAI           = "openai"
	providerAnthropic        = "anthropic"
	providerGeneric          = "generic"
)

type providerRoute struct {
	Name     string
	Upstream *url.URL
}

type providerContextKey struct{}

func providerFromContext(ctx context.Context) providerRoute {
	if route, ok := ctx.Value(providerContextKey{}).(providerRoute); ok && route.Upstream != nil {
		return route
	}
	return providerRoute{}
}

func withProviderRoute(r *http.Request, route providerRoute) *http.Request {
	ctx := context.WithValue(r.Context(), providerContextKey{}, route)
	return r.WithContext(ctx)
}

func buildProviderRoutes(cfg Config) (map[string]providerRoute, error) {
	cfg = cfg.withDefaults()
	if len(cfg.ProviderRoutes) == 0 {
		return nil, errors.New("at least one provider route is required")
	}

	routes := make(map[string]providerRoute, len(cfg.ProviderRoutes))
	for name, rawURL := range cfg.ProviderRoutes {
		name = normalizeProviderName(name)
		upstream, err := parseUpstreamURL(rawURL)
		if err != nil {
			return nil, err
		}
		routes[name] = providerRoute{Name: name, Upstream: upstream}
	}
	return routes, nil
}

func selectProviderRoute(r *http.Request, defaultProvider string, routes map[string]providerRoute) (providerRoute, bool) {
	name := normalizeProviderName(r.Header.Get(tokenGuardProviderHeader))
	if name == "" {
		name = inferProviderFromPath(r.URL.Path)
	}
	if name == "" {
		name = normalizeProviderName(defaultProvider)
	}
	if route, ok := routes[name]; ok {
		return route, true
	}
	return providerRoute{}, false
}

func inferProviderFromPath(path string) string {
	path = strings.ToLower(strings.TrimSpace(path))
	if strings.HasPrefix(path, "/v1/messages") || strings.HasPrefix(path, "/messages") {
		return providerAnthropic
	}
	return ""
}

func normalizeProviderName(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	provider = strings.ReplaceAll(provider, "_", "-")
	if provider == "" {
		return ""
	}
	switch provider {
	case "claude":
		return providerAnthropic
	case "openai-compatible", "openrouter", "groq", "mistral", "together", "fireworks", "deepseek":
		return provider
	default:
		return provider
	}
}

func providerKind(provider string) string {
	switch normalizeProviderName(provider) {
	case providerOpenAI, "openai-compatible", "openrouter", "groq", "mistral", "together", "fireworks", "deepseek":
		return providerOpenAI
	case providerAnthropic:
		return providerAnthropic
	default:
		return providerGeneric
	}
}
