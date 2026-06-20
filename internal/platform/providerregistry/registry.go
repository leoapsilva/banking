// Package providerregistry resolves a BaaS-specific implementation of a
// feature's port (e.g. checkout.CheckoutProvider) given the "baas" value a
// client sends in the unified API payload. It keeps every feature decoupled
// from which concrete BaaS adapters exist.
package providerregistry

import "fmt"

// Registry stores provider implementations keyed by (feature, baas).
type Registry struct {
	entries map[string]any
}

// New creates an empty registry.
func New() *Registry {
	return &Registry{entries: make(map[string]any)}
}

func key(feature, baas string) string {
	return feature + ":" + baas
}

// Register associates a feature+baas pair with a concrete provider
// implementation. Call this from main.go during dependency wiring.
func Register(r *Registry, feature, baas string, impl any) {
	r.entries[key(feature, baas)] = impl
}

// Get resolves the provider implementation for a feature+baas pair and
// type-asserts it to T. Returns an error if nothing is registered or the
// registered value does not match T.
func Get[T any](r *Registry, feature, baas string) (T, error) {
	var zero T
	raw, ok := r.entries[key(feature, baas)]
	if !ok {
		return zero, fmt.Errorf("providerregistry: no provider registered for feature=%q baas=%q", feature, baas)
	}
	typed, ok := raw.(T)
	if !ok {
		return zero, fmt.Errorf("providerregistry: provider for feature=%q baas=%q has unexpected type", feature, baas)
	}
	return typed, nil
}
