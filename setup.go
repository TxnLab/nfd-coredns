package nfd_coredns

import (
	"strconv"
	"strings"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	"github.com/coredns/caddy"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"

	"github.com/TxnLab/nfd-coredns/internal/nfd"
)

func init() {
	plugin.Register(pluginName, setupNFD)
}

func setupNFD(c *caddy.Controller) error {
	node, token, registryID, nfdNameServers, err := nfdParse(c)
	if err != nil {
		return plugin.Error(pluginName, err)
	}
	algoClient, err := algod.MakeClient(node, token)
	if err != nil {
		return plugin.Error(pluginName, err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return &NfdPlugin{
			Next:           next,
			NfdCache:       nfd.NewNfdCache(algoClient, registryID),
			nfdNameServers: nfdNameServers,
		}
	})

	return nil
}

func nfdParse(c *caddy.Controller) (string, string, uint64, []string, error) {
	var (
		node       string
		token      string
		registryID uint64 = 760937186 // mainnet
		err        error
	)
	nfdNameServers := make([]string, 0)

	c.Next()
	for c.NextBlock() {
		switch strings.ToLower(c.Val()) {
		case "node":
			args := c.RemainingArgs()
			if len(args) == 0 {
				return "", "", 0, nil, c.Errf("invalid node; no value")
			}
			if len(args) > 1 {
				return "", "", 0, nil, c.Errf("invalid node; multiple values")
			}
			node = args[0]
		case "token":
			args := c.RemainingArgs()
			if len(args) == 0 {
				return "", "", 0, nil, c.Errf("invalid token; no value")
			}
			if len(args) > 1 {
				return "", "", 0, nil, c.Errf("invalid token; multiple values")
			}
			token = args[0]
		case "registryid":
			args := c.RemainingArgs()
			if len(args) == 0 {
				return "", "", 0, nil, c.Errf("invalid registryid; no value")
			}
			if len(args) > 1 {
				return "", "", 0, nil, c.Errf("invalid registryid; multiple values")
			}
			registryID, err = strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return "", "", 0, nil, c.Errf("invalid integer value for registry id")
			}
		case "nameservers":
			args := c.RemainingArgs()
			if len(args) == 0 {
				return "", "", 0, nil, c.Errf("invalid nameservers; no value")
			}
			nfdNameServers = make([]string, len(args))
			copy(nfdNameServers, args)
		default:
			return "", "", 0, nil, c.Errf("unknown value %v", c.Val())
		}
	}
	if node == "" {
		return "", "", 0, nil, c.Errf("no node")
	}
	if len(nfdNameServers) == 0 {
		return "", "", 0, nil, c.Errf("no nameservers")
	}
	for i := range nfdNameServers {
		if !strings.HasSuffix(nfdNameServers[i], ".") {
			nfdNameServers[i] = nfdNameServers[i] + "."
		}
	}
	log.Infof("node: %s, token: %s, registryID: %d, nameservers: %v", node, token, registryID, nfdNameServers)
	return node, token, registryID, nfdNameServers, nil
}
