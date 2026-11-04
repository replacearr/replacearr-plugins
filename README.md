# replacearr-plugins

Central marketplace catalog for [Replacearr](https://github.com/replacearr/replacearr) Go plugins.
Site: [replacearr.com](https://replacearr.com).

Replacearr fetches a single published index (not a git submodule):

```
https://raw.githubusercontent.com/replacearr/replacearr-plugins/main/index.json
```

Override with `REPLACEARR_PLUGIN_CATALOG_URL` if you host a fork.

## Layout

```
plugins/
  webhook-notifier/     # first-party notification plugin
  jellyfin-notifier/    # Jellyfin library refresh + inbound webhook secret
  openvpn-proxy/        # first-party OpenVPN/SOCKS plugin
  http-proxy/           # first-party HTTP/HTTPS/SOCKS proxy
  sample-indexer/       # demo Torznab indexer
  community/            # third-party plugins via git submodules
schemas/plugin.schema.json
scripts/generate-index.go
index.json              # CI-generated catalog consumed by Replacearr
```

Each plugin directory contains:

- `plugin.json` — marketplace metadata (source of truth for CI)
- `main.go` — `pluginsdk.Serve(...)` entrypoint (`CGO_ENABLED=0`)
- `go.mod` — depends only on `github.com/replacearr/replacearr/pkg/pluginsdk` (+ stdlib)

## `index.json` contract

```json
{
  "schemaVersion": 1,
  "updatedAt": "2026-11-04T00:00:00Z",
  "plugins": [
    {
      "id": "webhook-notifier",
      "name": "Webhook Notifier",
      "version": "0.1.0",
      "author": "Replacearr",
      "description": "...",
      "types": ["notification"],
      "homepage": "...",
      "repository": "...",
      "minReplacearrVersion": "0.1.0",
      "assets": [
        {
          "goos": "linux",
          "goarch": "amd64",
          "url": "https://github.com/replacearr/replacearr-plugins/releases/download/.../webhook-notifier_linux_amd64",
          "sha256": "...",
          "size": 1234567
        }
      ]
    }
  ]
}
```

Replacearr installs the asset matching `runtime.GOOS` / `runtime.GOARCH` after verifying `sha256`. Assets without checksums are rejected.

Until the first tagged release, `assets` may be empty — the Plugins UI still lists catalog entries, but Install requires published binaries.

## Add a first-party plugin

1. Create `plugins/<slug>/` with `plugin.json`, `main.go`, and `go.mod`.
2. Implement `pluginsdk.Plugin` (+ capability interfaces) and call `pluginsdk.Serve`.
3. Open a PR. CI validates `plugin.json` and builds the plugin.
4. Tag a release:
   - Single plugin: `git tag webhook-notifier-v0.1.0 && git push --tags`
   - All plugins: `git tag v0.1.0 && git push --tags`

The release workflow cross-compiles, uploads GitHub Release assets, regenerates `index.json`, and pushes it to `main`.

## Add a community plugin (submodule)

```bash
git submodule add https://github.com/example/my-replacearr-plugin.git plugins/community/my-plugin
```

The submodule root must contain a valid `plugin.json` and a buildable Go `main`. CI treats community plugins the same as first-party ones.

## Local index generation

```bash
go run ./scripts/generate-index.go -root . -out index.json
# With release checksums:
go run ./scripts/generate-index.go -root . -assets .artifacts/checksums.json -out index.json
```

## SDK dependency

Plugin modules import `github.com/replacearr/replacearr/pkg/pluginsdk` directly. No `replace` directive is required.
