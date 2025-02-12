# nix-binary-cache

> An easy-to-deploy and maintain NixOS binary cache service

Many existing NixOS binary cache services, such as [nix-serve](https://github.com/edolstra/nix-serve), [nix-serve-ng](https://github.com/aristanetworks/nix-serve-ng), [attic](https://github.com/zhaofengli/attic), etc., require Nix for their own deployment. I find this quite absurd. Deploying a NixOS cache service should have a simpler and more universal approach.

nix-binary-cache is an extremely simple NixOS binary cache service.

- It can configure multiple upstream mirror sources and set up independent proxies for each.
- It maintains a cache on the local filesystem to avoid repeated access to upstream mirrors.
- It supports uploading built packages to the filesystem cache on NixOS.
- It is implemented in Go and can be easily deployed using Docker Compose!

## Quick Start

Prepare the configuration file and Docker Compose declaration file.

```sh
git clone https://github.com/117503445/nix-binary-cache.git
cd nix-binary-cache/docs/example
```

Start the service.

```sh
docker compose up -d
```

### Using the Mirror

Include the following in `flake.nix`:

```nix
nixConfig = {
  require-sigs = false;
};
```

When executing `switch`, specify the cache service address via `--option substituters` to use the cache service.

```sh
nixos-rebuild switch --accept-flake-config --print-build-logs --show-trace --option substituters http://SERVER_IP:8080
```

### Using the Mirror and Pushing Cache [Recommended]

Include the following in `flake.nix`:

```nix
nixConfig = {
  require-sigs = false;
  post-build-hook = ./scripts/upload-to-cache.sh;
};
```

The content of `./scripts/upload-to-cache.sh` is:

```sh
#!/bin/sh

# https://nix.dev/manual/nix/2.19/advanced-topics/post-build-hook
set -eu

echo "Uploading to cache, paths: $OUT_PATHS"
echo "NIX_CACHE_URL: $NIX_CACHE_URL"

# if "ustc" in $NIX_CACHE_URL, skip
if echo "$NIX_CACHE_URL" | grep -q "ustc"; then
    echo "Skip uploading to cache"
    exit 0
fi

nix copy --no-check-sigs --to $NIX_CACHE_URL $OUT_PATHS
```

When executing `switch`, the `upload-to-cache.sh` script will be triggered after each package build, uploading the built package to the cache service via `nix copy`.

```sh
NIX_CACHE_URL=http://SERVER_IP:8080 nixos-rebuild switch --accept-flake-config --print-build-logs --show-trace --option substituters http://SERVER_IP:8080
```

## Configuration Reference

Using a TOML format configuration file, take the configuration in the quick start as an example:

```toml
[[upstreams]]
url = "https://mirrors.ustc.edu.cn/nix-channels/store"

[[upstreams]]
url = "https://mirrors.tuna.tsinghua.edu.cn/nix-channels/store"

[[upstreams]]
url = "https://cache.nixos.org"
proxy = "http://host.docker.internal:1080"
```

Multiple upstreams are configured in order. Each upstream has a mandatory `url` and an optional `proxy`. When `proxy` is empty, it defaults to using a direct connection. When `proxy` is not empty, it connects to the upstream mirror source via an HTTP/SOCKS5 proxy server.

## Implementation

After setting `substituters`, the HTTP API calls initiated by Nix can be simplified as:

```plaintext
PUT $PATH with $CONTENT: Write $CONTENT to a certain HTTP Path
GET $PATH: Read $CONTENT from a certain HTTP Path
```

`nix-binary-cache` maps a $PATH to a filesystem path using SHA256. It then handles PUT and GET requests from Nix:

- For PUT requests, if the corresponding cache path does not exist, create the file and write the content.
- For GET requests:
    - If the corresponding cache path exists, return the corresponding content and 200.
    - If the corresponding cache path does not exist, sequentially access the upstream mirrors. If it exists, return the corresponding content and 200, and write to the cache path.
    - If none of the upstream mirrors exist, return 404.

Note that Nix's HTTP API actual calling rules are more complex, with certain PATHs having special semantics, and they do not seem to be standardized or documented. However, in my own usage, this implementation works flawlessly.

## References

<https://www.rectcircle.cn/posts/nix-4-http-binary-cache/>
