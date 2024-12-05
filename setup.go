package nfd_coredns

import (
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	"github.com/coredns/caddy"

	"github.com/TxnLab/nfd-coredns/internal/nfd"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/forward"
	"github.com/coredns/coredns/plugin/pkg/proxy"
	"github.com/coredns/coredns/plugin/pkg/transport"
)

const (
	// defaults
	defRegId        = 760937186
	defAlgoXyzIp    = "34.111.170.195"
	defCacheMinutes = 5
)

func init() {
	plugin.Register(pluginName, setupNfd)
}

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
		forwarder := forward.New()
		forwarder.Next = next
		forwarder.SetProxy(proxy.NewProxy("forward", "1.1.1.1:53", transport.DNS))

		return &NfdPlugin{
			Next:      next,
			Forwarder: forwarder,
			NfdCache: nfd.NewNfdCache(
				algoClient,
				pluginCfg.registryID,
				pluginCfg.algoXyzIp,
				time.Duration(pluginCfg.cacheMins)*time.Minute,
			),
			nfdNameServers: pluginCfg.nfdNameServers,
		}
	})

	return nil
}

type nfdPluginConfig struct {
	nodeUrl        string
	token          string
	registryID     uint64
	nfdNameServers []string
	algoXyzIp      string
	cacheMins      int
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
	nfdNameServers := make([]string, 0)

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
		case "nameservers":
			args := c.RemainingArgs()
			if len(args) == 0 {
				return nil, c.Errf("invalid nameservers; no value")
			}
			nfdNameServers = make([]string, len(args))
			copy(nfdNameServers, args)
		default:
			return nil, c.Errf("unknown value %v", c.Val())
		}
	}
	if node == "" {
		return nil, c.Errf("no node")
	}
	if len(nfdNameServers) == 0 {
		return nil, c.Errf("no nameservers")
	}
	for i := range nfdNameServers {
		if !strings.HasSuffix(nfdNameServers[i], ".") {
			nfdNameServers[i] = nfdNameServers[i] + "."
		}
	}
	log.Infof(
		"node: %s, token: %s, registryID: %d, nameservers: %v, algoXyzIp: %s, cacheMins: %d",
		node,
		token,
		registryID,
		nfdNameServers,
		algoXyzIp,
		cacheMins,
	)
	return &nfdPluginConfig{
		nodeUrl:        node,
		token:          token,
		registryID:     registryID,
		nfdNameServers: nfdNameServers,
		algoXyzIp:      algoXyzIp,
		cacheMins:      cacheMins,
	}, nil
}
