package sync

import (
	"context"
	"crypto/tls"
	"net"
	"net/url"
)

// dialConn opens a connection to u's host through d, wrapping in TLS (with
// the given ALPN protocol) only when u's scheme is https. Plain http targets
// (including the local h2c fixture used in Phase 1) get a raw TCP conn.
func dialConn(ctx context.Context, d Dialer, u *url.URL, alpn string) (net.Conn, error) {
	addr := u.Host
	if u.Port() == "" {
		port := "80"
		if u.Scheme == "https" {
			port = "443"
		}
		addr = net.JoinHostPort(u.Hostname(), port)
	}
	c, err := d.Dial(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" {
		return c, nil
	}
	tlsConn := tls.Client(c, &tls.Config{
		ServerName: u.Hostname(),
		NextProtos: []string{alpn},
		MinVersion: tls.VersionTLS12,
	})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = c.Close()
		return nil, err
	}
	return tlsConn, nil
}
