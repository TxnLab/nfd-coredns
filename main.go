/*
 * Copyright (c) 2024. TxnLab Inc.
 * All Rights reserved.
 */

package main

import (
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/coremain"
	_ "github.com/coredns/coredns/plugin/cache"
	_ "github.com/coredns/coredns/plugin/cancel"
	_ "github.com/coredns/coredns/plugin/debug"
	_ "github.com/coredns/coredns/plugin/errors"
	_ "github.com/coredns/coredns/plugin/file"
	_ "github.com/coredns/coredns/plugin/forward"
	_ "github.com/coredns/coredns/plugin/health"
	_ "github.com/coredns/coredns/plugin/log"
	_ "github.com/coredns/coredns/plugin/metrics"
	_ "github.com/coredns/coredns/plugin/reload"
	_ "github.com/coredns/coredns/plugin/rewrite"
)

var directives = []string{
	"root:root",
	"cancel",
	"reload",
	"debug",
	"health",
	"prometheus",
	"errors",
	"log",
	"cache",
	"rewrite",
	"file",
	"forward",
	"nfd",
}

func init() {
	dnsserver.Directives = directives
}

func main() {
	coremain.Run()
}
