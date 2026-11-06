package main

import (
	"net"
	"strings"
	"testing"
)

func TestBrowserAddress(t *testing.T) {
	for listen, want := range map[string]string{
		"127.0.0.1:2412": "http://127.0.0.1:2412",
		"0.0.0.0:2412":   "http://127.0.0.1:2412",
		"[::]:2412":      "http://127.0.0.1:2412",
		"[::1]:2412":     "http://[::1]:2412",
	} {
		address, err := net.ResolveTCPAddr("tcp", listen)
		if err != nil {
			t.Fatal(err)
		}
		if got := browserAddress(address); got != want {
			t.Errorf("%s: %s", listen, got)
		}
	}
}

func TestPlainBanner(t *testing.T) {
	// 重定向到文件时不能混入转义序列。
	var out strings.Builder
	printBanner(&out, "http://127.0.0.1:2412", false)
	if strings.Contains(out.String(), "\x1b") || !strings.Contains(out.String(), "打开 http://127.0.0.1:2412 以使用 WebUI") {
		t.Fatal(out.String())
	}
}
