# replacearr-plugins

Curated marketplace registry for [Replacearr](https://github.com/replacearr/replacearr) plugins.
Site: [replacearr.com](https://replacearr.com) · Discord: [discord.gg/WUrKJuE5XB](https://discord.gg/WUrKJuE5XB).

Replacearr fetches:

```
https://raw.githubusercontent.com/replacearr/replacearr-plugins/main/registry.json
```

Override with `REPLACEARR_PLUGIN_CATALOG_URL`.

## Trust model

- **Curated list** — `registry.json` points at **relative** `manifests/{name}.json` files in this repo. Those files pin exact release asset URLs and sha256 digests. Promoting a plugin is a PR that bumps the submodule and replaces the manifest (reviewers see checksums in-diff). Never use `/releases/latest` here.
- **Submodules** under `plugins/{name}` → `replacearr/plugin-{name}` pin reviewed source for browsing; they are not the HTTP install path (GitHub does not serve submodule contents via this repo’s raw URLs).
- **Paste-URL installs** — users can install any compatible GitHub plugin repo from the Replacearr UI without a registry PR. That path is explicitly untrusted / opt-in.

## Layout

```
registry.json              # curated id → relative manifest path
manifests/*.json           # pinned install metadata (assets + sha256)
plugins/{name}/            # git submodule → replacearr/plugin-{name}
schemas/
```

## Add / update a curated plugin

1. Release from `replacearr/plugin-{name}` (`git tag vX.Y.Z && git push --tags`).
2. In this repo: bump the submodule to that commit.
3. Copy the release `plugin.json` into `manifests/{name}.json`.
4. Ensure `registry.json` has `{ "id": "...", "manifest": "manifests/{name}.json" }`.
5. Open a PR.

## Community / third-party

Paste the GitHub repo URL in **Plugins → Marketplace**. To appear in the curated list, open a PR adding a reviewed `manifests/` entry (+ optional submodule).

## Plugin contract

Each `replacearr/plugin-{name}` repo tags `v*`, uploads platform binaries and a release `plugin.json` with:

```json
{
  "id": "…",
  "name": "…",
  "version": "0.1.0",
  "author": "…",
  "description": "…",
  "types": ["notification"],
  "homepage": "https://github.com/replacearr/plugin-…",
  "repository": "https://github.com/replacearr/plugin-…",
  "minReplacearrVersion": "0.1.0",
  "assets": [
    {
      "goos": "linux",
      "goarch": "amd64",
      "url": "https://github.com/replacearr/plugin-…/releases/download/v0.1.0/…_linux_amd64",
      "sha256": "…",
      "size": 1234567
    }
  ]
}
```
