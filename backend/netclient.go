package backend

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// CreateHTTPClient returns a client that goes through the proxy setting:
// one URL, or several separated by commas or spaces, taken in turn. An
// empty setting uses the environment's (HTTPS_PROXY, NO_PROXY…).
func CreateHTTPClient(proxySetting string, timeout time.Duration) (*http.Client, error) {
	proxy, err := proxyFunc(proxySetting)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = proxy
	return &http.Client{Transport: transport, Timeout: timeout}, nil
}

func proxyFunc(setting string) (func(*http.Request) (*url.URL, error), error) {
	fields := strings.FieldsFunc(setting, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' })
	if len(fields) == 0 {
		return http.ProxyFromEnvironment, nil
	}
	proxies := make([]*url.URL, 0, len(fields))
	for _, f := range fields {
		u, err := url.Parse(f)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("the proxy %q is not a URL such as http://127.0.0.1:7890", f)
		}
		proxies = append(proxies, u)
	}
	if len(proxies) == 1 {
		return http.ProxyURL(proxies[0]), nil
	}
	var next atomic.Uint64
	return func(*http.Request) (*url.URL, error) {
		return proxies[(next.Add(1)-1)%uint64(len(proxies))], nil
	}, nil
}
