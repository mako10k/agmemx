package cli

import (
	"encoding/json"
	"errors"
	"io"

	"agmemx/internal/embed"
	"agmemx/internal/store"
	"agmemx/internal/xdg"
)

func handleReindex(stdout, stderr io.Writer, roots xdg.Roots, opts options, resolved string, body map[string]json.RawMessage) int {
	if rej := unknownKeys(body); rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	if rej := providerReady(opts); rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	ids, err := store.IDsInSubtree(roots.Data, resolved)
	if err != nil {
		fmtErr(stderr, err)
		return 2
	}
	baseKey := cacheKey(opts, "")
	if err := embed.DropStage(roots.Cache, baseKey); err != nil {
		fmtErr(stderr, err)
		return 2
	}
	defer func() { _ = embed.DropStage(roots.Cache, baseKey) }()
	for _, id := range ids {
		obj, err := loadObject(roots.Data, id)
		if err != nil {
			fmtErr(stderr, err)
			return 2
		}
		vector, rej := embedText(roots.Cache, opts, obj.Text)
		if rej != nil {
			writeReject(stdout, rej)
			return rej.exit
		}
		putKey := baseKey
		putKey.Text = obj.Text
		if err := embed.StagePut(roots.Cache, putKey, vector); err != nil {
			if errors.Is(err, embed.ErrDimension) {
				writeReject(stdout, reject("embed_dimension_mismatch", 1))
				return 1
			}
			fmtErr(stderr, err)
			return 2
		}
	}
	if err := embed.PublishStage(roots.Cache, baseKey); err != nil {
		fmtErr(stderr, err)
		return 2
	}
	writeOK(stdout, struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Count    int    `json:"count"`
	}{Provider: opts.provider, Model: opts.model, Count: len(ids)})
	return 0
}
