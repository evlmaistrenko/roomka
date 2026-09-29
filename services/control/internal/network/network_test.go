package network

import (
	"net/http"
	"testing"
)

func TestClientAddress(t *testing.T) {
	cases := []struct {
		name      string
		peer      string
		forwarded []string
		want      string
	}{
		{"direct client", "203.0.113.7:5000", nil, "203.0.113.7"},
		{"direct client claiming another address", "203.0.113.7:5000", []string{"198.51.100.1"}, "203.0.113.7"},
		{"behind the local proxy", "127.0.0.1:5000", []string{"198.51.100.1"}, "198.51.100.1"},
		{"behind the proxy, client prepended a lie", "[::1]:5000", []string{"10.0.0.1, 198.51.100.1"}, "198.51.100.1"},
		{"behind the proxy, several headers", "127.0.0.1:5000", []string{"10.0.0.1", "198.51.100.1"}, "198.51.100.1"},
		{"local, nothing forwarded", "127.0.0.1:5000", nil, "127.0.0.1"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := &http.Request{RemoteAddr: testCase.peer, Header: http.Header{}}
			for _, value := range testCase.forwarded {
				request.Header.Add("X-Forwarded-For", value)
			}
			if got := ClientAddress(request); got != testCase.want {
				t.Errorf("ClientAddress = %q, want %q", got, testCase.want)
			}
		})
	}
}
