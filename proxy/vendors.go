package proxy

// VendorConfig describes how to reach an upstream AI vendor API.
type VendorConfig struct {
	UpstreamHost string
	AuthHeader   string
	AuthFormat   func(key string) string
}

// DefaultVendors returns a fresh map of the built-in vendor configurations.
// Each call returns a new map so callers may mutate it without affecting others.
func DefaultVendors() map[string]VendorConfig {
	return map[string]VendorConfig{
		"anthropic": {
			UpstreamHost: "api.anthropic.com",
			AuthHeader:   "x-api-key",
			AuthFormat:   func(key string) string { return key },
		},
		"openai": {
			UpstreamHost: "api.openai.com",
			AuthHeader:   "Authorization",
			AuthFormat:   func(key string) string { return "Bearer " + key },
		},
	}
}
