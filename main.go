package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/117503445/goutils"
	"github.com/alecthomas/kong"
	"github.com/rs/zerolog/log"
)

const cacheDir = "./cache/"

type upstream struct {
	URL      string
	UseProxy bool
}

func newHandle(httpProxy string, upstreams []upstream) (func(w http.ResponseWriter, r *http.Request), error) {
	client := http.DefaultClient
	proxyClient := http.DefaultClient
	if httpProxy != "" {
		httpProxyURL, err := url.Parse(httpProxy)
		if err != nil {
			return nil, err
		}
		proxyClient = &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(httpProxyURL)}}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		reqID := goutils.UUID4()
		ctx = log.With().Str("reqID", reqID).Logger().WithContext(ctx)

		log.Ctx(ctx).Debug().Str("path", r.URL.Path).Str("method", r.Method).Msg("request")

		cacheFilePath := cacheDir + fmt.Sprintf("%x", r.URL.String())
		if r.Method == "PUT" {
			cacheFile, err := os.Create(cacheFilePath)
			if err != nil {
				log.Ctx(ctx).Fatal().Err(err).Msg("failed to create cache file") // TODO: 500
				return
			}
			defer cacheFile.Close()

			_, err = io.Copy(cacheFile, r.Body)
			if err != nil {
				log.Ctx(ctx).Fatal().Err(err).Msg("failed to write cache file") // TODO: 500
				return
			}

			w.WriteHeader(200)
			return
		}

		// TODO 优先从缓存中读取。
		if _, err := os.Stat(cacheFilePath); err == nil {
			log.Ctx(ctx).Debug().Str("cacheFile", cacheFilePath).Msg("cache hit")
			// Cache hit: Serve the cached file
			http.ServeFile(w, r, cacheFilePath)
			return
		}

		// 尝试按顺序 upstream 中获取
		var resp *http.Response
		for _, u := range upstreams {
			targetPath := strings.TrimSuffix(u.URL, "/") + r.URL.Path
			c := client
			if u.UseProxy {
				c = proxyClient
			}
			_resp, err := c.Get(targetPath)
			if err != nil {
				log.Ctx(ctx).Debug().Err(err).Str("upstream", u.URL).Str("targetPath", targetPath).Msg("failed to get")
				continue
			}
			if _resp.StatusCode != 200 {
				log.Ctx(ctx).Debug().Str("upstream", u.URL).Str("targetPath", targetPath).Int("statusCode", _resp.StatusCode).Msg("failed to get")
				_resp.Body.Close()
				continue
			}
			log.Ctx(ctx).Debug().Str("upstream", u.URL).Str("targetPath", targetPath).Msg("success to get")
			resp = _resp
			break
		}
		if resp == nil {
			// log.Printf("  all upstream not found")
			log.Ctx(ctx).Warn().Msg("all upstream not found")
			w.WriteHeader(404)
			return
		}
		defer resp.Body.Close()
		// w.WriteHeader(resp.StatusCode)

		// write resp.Body to cachefile
		cacheFile, err := os.Create(cacheFilePath)
		if err != nil {
			log.Ctx(ctx).Fatal().Err(err).Msg("failed to create cache file") // TODO: 500
			return
		}
		defer cacheFile.Close()

		// TODO: write to temp file first
		_, err = io.Copy(cacheFile, resp.Body)
		if err != nil {
			log.Ctx(ctx).Fatal().Err(err).Msg("failed to write cache file") // TODO: 500
			return
		}

		// Serve the response to the client
		http.ServeFile(w, r, cacheFilePath)
	}, nil
}

type CmdServe struct {
}

func (cmd *CmdServe) Run() error {
	if err := os.MkdirAll(cacheDir, os.ModePerm); err != nil {
		log.Fatal().Err(err).Msg("failed to create cache dir")
	}

	upstreams := []upstream{
		{
			URL:      "https://mirror.sjtu.edu.cn/nix-channels/store",
			UseProxy: false,
		},
		{
			URL:      "https://mirrors.tuna.tsinghua.edu.cn/nix-channels/store",
			UseProxy: false,
		},
		{
			URL:      "https://cache.nixos.org",
			UseProxy: true,
		},
	}
	httpProxy := os.Getenv("HTTP_PROXY")
	os.Unsetenv("HTTP_PROXY")
	handle, err := newHandle(httpProxy, upstreams)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create handle")
	}
	http.HandleFunc("/", handle)
	log.Info().Int("port", 8000).Msg("listen")
	if err := http.ListenAndServe(":8000", nil); err != nil {
		log.Fatal().Err(err).Msg("failed to listen")
	}
	return nil
}

type CmdList struct {
}

func (cmd *CmdList) Run() error {
	files, err := os.ReadDir(cacheDir)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to read cache dir")
	}
	for _, file := range files {
		name := file.Name()
		// convert from hex to string
		fileName , err:= hex.DecodeString(name)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to decode file name")
		}

		log.Info().Str("file", string(fileName)).Msg("")
	}
	return nil
}

func main() {
	goutils.InitZeroLog(goutils.WithProduction{})

	var cli struct {
		Serve CmdServe `cmd:"" help:"start a server"`
		List  CmdList  `cmd:"list" help:"list all cache files"`
	}
	ctx := kong.Parse(&cli)
	err := ctx.Run()
	ctx.FatalIfErrorf(err)
}
