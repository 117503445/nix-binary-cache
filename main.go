package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/117503445/goutils"
	"github.com/alecthomas/kong"
	kongtoml "github.com/alecthomas/kong-toml"
	"github.com/rs/zerolog/log"
)

func UrlToPath(url string) string {
	// use sha256
	return fmt.Sprintf("%x", sha256.Sum256([]byte(url)))
}

type fetcher struct {
	clients []*http.Client
	urls    []string
}

func NewFetcher(upstreams []Upstream) *fetcher {
	clients := []*http.Client{}
	urls := []string{}
	for _, upstream := range upstreams {
		var client *http.Client
		if upstream.Proxy != "" {
			httpProxyURL, err := url.Parse(upstream.Proxy)
			if err != nil {
				log.Fatal().Err(err).Msg("failed to parse proxy url")
			}
			client = &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(httpProxyURL)}}
		} else {
			client = http.DefaultClient
		}

		clients = append(clients, client)
		urls = append(urls, strings.TrimSuffix(upstream.Url, "/"))
	}

	return &fetcher{clients: clients, urls: urls}
}

func (f *fetcher) Fetch(path string) *http.Response {
	logger := log.With().Str("path", path).Logger()

	for i, client := range f.clients {
		uLogger := logger.With().Str("upstream", f.urls[i]).Logger()

		url := f.urls[i] + path
		resp, err := client.Get(url)
		if err != nil {
			uLogger.Debug().Err(err).Msg("request error")
			continue
		}
		if resp.StatusCode != 200 {
			uLogger.Debug().Int("statusCode", resp.StatusCode).Msg("request fail")
			continue
		}
		uLogger.Debug().Msg("request success")
		return resp
	}
	logger.Debug().Msg("all upstream not found")
	return nil
}

func newHandle(fetcher *fetcher) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := log.With().Str("reqID", goutils.UUID4()).Logger()
		logger.Debug().Str("path", r.URL.Path).Str("method", r.Method).Msg("request")

		cacheFilePath := fmt.Sprintf("%s/%s", cli.CacheDir, UrlToPath(r.URL.Path))

		// TODO: auth
		if r.Method == "PUT" {
			err := goutils.AtomicWriteFile(cacheFilePath, r.Body)
			if err != nil {
				logger.Warn().Err(err).Msg("failed to write cache file")
				w.WriteHeader(500)
				return
			}
			logger.Debug().Str("cacheFile", cacheFilePath).Msg("cache written")
			w.WriteHeader(200)
			return
		}

		if _, err := os.Stat(cacheFilePath); err == nil {
			logger.Debug().Str("cacheFile", cacheFilePath).Msg("cache hit")
			// Cache hit: Serve the cached file
			http.ServeFile(w, r, cacheFilePath)
			return
		}

		resp := fetcher.Fetch(r.URL.Path)
		if resp == nil {
			logger.Debug().Msg("all upstream not found")
			w.WriteHeader(404)
			return
		}

		defer resp.Body.Close()
		// TODO: Garbage collection
		err := goutils.AtomicWriteFile(cacheFilePath, resp.Body)
		if err != nil {
			logger.Warn().Err(err).Msg("failed to write cache file")
			w.WriteHeader(500)
			return
		}
		logger.Debug().Msg("success to fetch")
		// Serve the response to the client
		http.ServeFile(w, r, cacheFilePath)
	}
}

type CmdServe struct {
}

func (cmd *CmdServe) Run() error {
	if err := os.MkdirAll(cli.CacheDir, os.ModePerm); err != nil {
		log.Fatal().Err(err).Msg("failed to create cache dir")
	}

	fetcher := NewFetcher(cli.Upstreams)
	http.HandleFunc("/", newHandle(fetcher))

	log.Info().Int("port", 8000).Msg("listen")
	if err := http.ListenAndServe(":8000", nil); err != nil {
		log.Fatal().Err(err).Msg("failed to listen")
	}
	return nil
}

type CmdList struct {
}

func (cmd *CmdList) Run() error {
	files, err := os.ReadDir(cli.CacheDir)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to read cache dir")
	}
	for _, file := range files {
		name := file.Name()
		// convert from hex to string
		fileName, err := hex.DecodeString(name)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to decode file name")
		}

		log.Info().Str("file", string(fileName)).Msg("")
	}
	return nil
}

var cli struct {
	Serve CmdServe `cmd:"serve" help:"start a server"`
	List  CmdList  `cmd:"list" help:"list all cache files"`

	CacheDir string `help:"cache dir" default:"./cache"`
	LogDir   string `help:"log dir" default:"./logs"`

	Upstreams []Upstream `help:"upstreams"`
}

type Upstream struct {
	Url   string
	Proxy string
}

func main() {
	ctx := kong.Parse(&cli, kong.Configuration(kongtoml.Loader, "/workspace/config.toml"))
	cli.CacheDir = strings.TrimSuffix(cli.CacheDir, "/")

	goutils.InitZeroLog(goutils.WithProduction{
		DirLog: cli.LogDir,
	})
	log.Debug().Interface("cli", cli).Msg("")

	err := ctx.Run()
	ctx.FatalIfErrorf(err)
}
