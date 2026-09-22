package cli

import "agmemx/internal/embed"

func embedText(cacheHome string, opts options, text string) ([]float64, *rejection) {
	if opts.provider == "" || opts.model == "" {
		return nil, reject("embed_provider_unset", 1)
	}
	switch opts.provider {
	case "fixture":
		if opts.fixture == "" {
			return nil, reject("embed_provider_unset", 1)
		}
		vec, err := embed.FixtureVector(opts.fixture, opts.model, text)
		if err != nil {
			return nil, reject("embed_fixture_invalid", 1)
		}
		return vec, nil
	default:
		return nil, reject("embed_unreachable", 1)
	}
}

func cacheKey(opts options, text string) embed.Key {
	return embed.Key{Provider: opts.provider, BaseURL: opts.baseURL, Model: opts.model, Text: text}
}
