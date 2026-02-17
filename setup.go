/*
 * Copyright (c) 2024. TxnLab Inc.
 * All Rights reserved.
 */

package main

import (
	"embed"
	"io/fs"
	"maps"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	"github.com/coredns/caddy"
	"github.com/miekg/dns"

	"github.com/coredns/coredns/plugin/file"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/forward"
	"github.com/coredns/coredns/plugin/pkg/proxy"
	"github.com/coredns/coredns/plugin/pkg/transport"

	"github.com/TxnLab/nfd-coredns/internal/nfd"
)

const (
	// defaults
	defRegId        = 760937186
	defAlgoXyzIp    = "34.8.101.7"
	defCacheMinutes = 5
)

func init() {
	plugin.Register(pluginName, setupNfd)
}

// Now embed in the binary the root zone definitions for algo.xyz and dotalgo.io (testing)
// since the NS records will point to our service yet there are certain 'root' entries
// we'll need to serve (like A record for algo.xyz for eg)
//
//go:embed internal/zones
var embeddedZones embed.FS

func setupNfd(c *caddy.Controller) error {
	pluginCfg, err := nfdParse(c)
	if err != nil {
		return plugin.Error(pluginName, err)
	}
	algoClient, err := algod.MakeClient(pluginCfg.nodeUrl, pluginCfg.token)
	if err != nil {
		return plugin.Error(pluginName, err)
	}
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		var (
			zoneConfig = map[string]*file.Zone{}
			zoneSOA    dns.RR // SOA for negative responses (RFC 2308)
		)
		fs.WalkDir(embeddedZones, "internal/zones", func(path string, d fs.DirEntry, err error) error {
			if d.IsDir() {
				return nil
			}
			origin := d.Name() + "."
			log.Info(path)
			zoneFile, err := embeddedZones.Open(path)
			if err != nil {
				log.Fatalf("failed to open embedded zone %s: %v", path, err)
			}
			parsedZone, err := file.Parse(zoneFile, origin, d.Name(), 0)
			if err != nil {
				log.Fatalf("failed to parse zone: %v", err)
			}
			zoneConfig[origin] = parsedZone

			// Extract SOA from algo.xyz zone for use in negative responses
			if origin == "algo.xyz." {
				if apex, apexErr := parsedZone.ApexIfDefined(); apexErr == nil && len(apex) > 0 {
					zoneSOA = apex[0] // SOA is first record
				}
			}
			return nil
		})
		filePlugin := file.File{
			Next: next,
			Zones: file.Zones{
				Z:     zoneConfig,
				Names: slices.Collect(maps.Keys(zoneConfig))},
		}

		// Initialize a Forwarder plugin we can use to handle out of zone cname->xxx lookups (using cloudflare)
		// Passed to nfd plugin to use selectively
		forwarder := forward.New()
		forwarder.Next = next
		forwarder.SetProxy(proxy.NewProxy("forward", "1.1.1.1:53", transport.DNS))

		nfdPlugin := &NfdPlugin{
			Next:      filePlugin,
			Forwarder: forwarder,
			NfdHandler: nfd.NewNfdRRHandler(
				algoClient,
				pluginCfg.registryID,
				pluginCfg.algoXyzIp,
				time.Duration(pluginCfg.cacheMins)*time.Minute,
			),
			zoneSOA:    zoneSOA,
			zoneOrigin: dnsserver.GetConfig(c).Zone,
		}

		return nfdPlugin
	})

	return nil
}

type nfdPluginConfig struct {
	nodeUrl    string
	token      string
	registryID uint64
	algoXyzIp  string
	cacheMins  int
}

func nfdParse(c *caddy.Controller) (*nfdPluginConfig, error) {
	var (
		node       string
		token      string
		registryID uint64 = defRegId
		algoXyzIp  string = defAlgoXyzIp
		cacheMins  int    = defCacheMinutes
		err        error
	)

	c.Next()
	for c.NextBlock() {
		switch strings.ToLower(c.Val()) {
		case "node":
			args := c.RemainingArgs()
			if len(args) == 0 {
				return nil, c.Errf("invalid node; no value")
			}
			if len(args) > 1 {
				return nil, c.Errf("invalid node; multiple values")
			}
			node = args[0]
		case "token":
			args := c.RemainingArgs()
			if len(args) == 0 {
				return nil, c.Errf("invalid token; no value")
			}
			if len(args) > 1 {
				return nil, c.Errf("invalid token; multiple values")
			}
			token = args[0]
		case "algoxyzip":
			args := c.RemainingArgs()
			if len(args) == 0 {
				return nil, c.Errf("invalid algoxyzip; no value")
			}
			if len(args) > 1 {
				return nil, c.Errf("invalid algoxyzip; multiple values")
			}
			algoXyzIp = args[0]
			ip := net.ParseIP(algoXyzIp)
			if ip == nil || ip.To4() == nil {
				return nil, c.Errf("invalid algoxyzip; not a valid IPv4 address")
			}
		case "registryid":
			args := c.RemainingArgs()
			if len(args) == 0 {
				return nil, c.Errf("invalid registryid; no value")
			}
			if len(args) > 1 {
				return nil, c.Errf("invalid registryid; multiple values")
			}
			registryID, err = strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return nil, c.Errf("invalid integer value for registry id")
			}
		case "cachemins":
			args := c.RemainingArgs()
			if len(args) == 0 {
				return nil, c.Errf("invalid cachemins; no value")
			}
			if len(args) > 1 {
				return nil, c.Errf("invalid cachemins; multiple values")
			}
			cacheMins, err = strconv.Atoi(args[0])
			if err != nil {
				return nil, c.Errf("invalid integer value for cache minutes")
			}
		default:
			return nil, c.Errf("unknown value %v", c.Val())
		}
	}
	if node == "" {
		return nil, c.Errf("no node")
	}
	log.Infof(
		"node: %s, token: %s, registryID: %d, algoXyzIp: %s, cacheMins: %d",
		node,
		token,
		registryID,
		algoXyzIp,
		cacheMins,
	)
	return &nfdPluginConfig{
		nodeUrl:    node,
		token:      token,
		registryID: registryID,
		algoXyzIp:  algoXyzIp,
		cacheMins:  cacheMins,
	}, nil
}
