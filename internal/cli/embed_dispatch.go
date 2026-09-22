package cli

import (
	"errors"
	"os"

	"agmemx/internal/embed"
)

func embedText(_ string, opts options, text string) ([]float64, *rejection) {
	if rej := providerReady(opts); rej != nil {
		return nil, rej
	}
	switch opts.provider {
	case "fixture":
		vec, err := embed.FixtureVector(opts.fixture, opts.model, text)
		if err != nil {
			return nil, reject("embed_fixture_invalid", 1)
		}
		return vec, nil
	case "ollama":
		vec, err := embed.Ollama(embed.EffectiveBase(opts.provider, opts.baseURL), opts.model, text)
		return hosted(vec, err)
	case "openai":
		key, rej := openAIKey(opts)
		if rej != nil {
			return nil, rej
		}
		vec, err := embed.OpenAI(embed.EffectiveBase(opts.provider, opts.baseURL), opts.model, text, key)
		return hosted(vec, err)
	default:
		return nil, reject("embed_unreachable", 1)
	}
}

func providerReady(opts options) *rejection {
	if opts.provider == "" || opts.model == "" {
		return reject("embed_provider_unset", 1)
	}
	switch opts.provider {
	case "fixture":
		if opts.fixture == "" {
			return reject("embed_provider_unset", 1)
		}
	case "openai":
		if _, rej := openAIKey(opts); rej != nil {
			return rej
		}
	}
	return nil
}

func openAIKey(opts options) (string, *rejection) {
	name := opts.keyEnv
	if name == "" {
		name = "OPENAI_API_KEY"
	}
	key := os.Getenv(name)
	if key == "" {
		return "", reject("embed_provider_unset", 1)
	}
	return key, nil
}

func hosted(vec []float64, err error) ([]float64, *rejection) {
	if err == nil {
		return vec, nil
	}
	if errors.Is(err, embed.ErrUnreachable) {
		return nil, reject("embed_unreachable", 1)
	}
	return nil, reject("embed_rejected", 1)
}

func cacheKey(opts options, text string) embed.Key {
	return embed.Key{
		Provider: opts.provider,
		BaseURL:  embed.EffectiveBase(opts.provider, opts.baseURL),
		Model:    opts.model,
		Text:     text,
	}
}
