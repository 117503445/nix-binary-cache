# nix-binary-cache

> 易于部署运维的 NixOS 二进制缓存

现有的诸多 NixOS 二进制缓存服务，如 [nix-serve](https://github.com/edolstra/nix-serve), [nix-serve-ng](https://github.com/aristanetworks/nix-serve-ng), [attic](https://github.com/zhaofengli/attic) 等，其本身的部署也需要 Nix。我觉得这实在太离谱了，部署 NixOS 缓存服务应当有更加简单、通用的方法。

nix-binary-cache 就是一款极简的 NixOS 二进制缓存服务

- 它可以配置多个上游镜像源，并为每个镜像源设置独立的代理
- 它在本地文件系统维护缓存，避免重复访问上游镜像源
- 它支持 NixOS 上传构建完成的包至文件系统缓存
- 它使用 Go 实现，可以非常轻松地使用 Docker Compose 部署！

## 快速开始

准备配置文件和 Docker Compose 声明文件

```sh
git clone https://github.com/117503445/nix-binary-cache.git
cd nix-binary-cache/docs/example
```

启动服务

```sh
docker compose up -d
```

### 使用镜像

在 `flake.nix` 中包含

```nix
nixConfig = {
  require-sigs = false;
};
```

执行 `switch` 时，通过 `--option substituters` 指定缓存服务地址，即可使用缓存服务

```sh
nixos-rebuild switch --accept-flake-config --print-build-logs --show-trace --option substituters http://SERVER_IP:8080
```

### 使用镜像并推送缓存[推荐]

在 `flake.nix` 中包含

```nix
nixConfig = {
  require-sigs = false;
  post-build-hook = ./scripts/upload-to-cache.sh;
};
```

`./scripts/upload-to-cache.sh` 的内容为

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

执行 `switch` 时，每完成一个包的构建都会触发 `upload-to-cache.sh` 脚本，通过 `nix copy` 将构建完成的包上传至缓存服务

```sh
NIX_CACHE_URL=http://SERVER_IP:8080 nixos-rebuild switch --accept-flake-config --print-build-logs --show-trace --option substituters http://SERVER_IP:8080
```

## 配置参考

使用 TOML 格式配置文件，以快速开始中的配置为例

```toml
[[upstreams]]
url = "https://mirrors.ustc.edu.cn/nix-channels/store"

[[upstreams]]
url = "https://mirrors.tuna.tsinghua.edu.cn/nix-channels/store"

[[upstreams]]
url = "https://cache.nixos.org"
proxy = "http://host.docker.internal:1080"
```

按顺序配置了多个 upstreams。每个 upstreams 除了必要的 `url`，还有可选的 `proxy`。当 `proxy` 配置为空时，默认是使用直接连接。当 `proxy` 配置不为空时，则通过 HTTP/SOCKS5
等代理服务器连接上游镜像源。

## 实现

设置 `substituters` 后，Nix 发起的 HTTP API 调用可以简化为

```plaintext
PUT $PATH with $CONTENT: 将 $CONTENT 写入某个 HTTP Path
GET $PATH: 读取某个 HTTP Path 的 $CONTENT
```

`nix-binary-cache` 通过 SHA256 将某个 $PATH 映射到文件系统中的一个路径。然后分别处理来自 Nix 的 PUT 和 GET 请求

- 对于 PUT 请求，如果对应缓存路径不存在，则创建文件并写入内容
- 对于 GET 请求
    - 如果对应缓存路径存在，则返回对应内容和 200
    - 如果对应缓存路径不存在，则依次访问上游镜像源，如果存在则返回对应内容和 200，并写入缓存路径
    - 如果上游镜像源都不存在，则返回 404

需要注意的是，Nix 的 HTTP API 实际调用规则更为复杂，其中某些 PATH 具有特殊语义，而且似乎没有标准化并形成文档。但我自己用下来，这个实现没一点毛病。

## 参考

<https://www.rectcircle.cn/posts/nix-4-http-binary-cache/>
