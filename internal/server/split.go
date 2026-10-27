package server

import (
	"bufio"
	"errors"
	"net"
	"sync"
	"time"
)

// tlsHandshake is the first byte of every TLS connection (a handshake record).
const tlsHandshake = 0x16

// connMux accepts connections on one port and routes TLS connections to the
// HTTPS server and everything else to the plain-HTTP redirector, so both
// https:// and http:// URLs work on the same port.
type connMux struct {
	ln    net.Listener
	tls   *chanListener
	plain *chanListener
}

func newConnMux(ln net.Listener) *connMux {
	return &connMux{
		ln:    ln,
		tls:   newChanListener(ln.Addr()),
		plain: newChanListener(ln.Addr()),
	}
}

// serve accepts connections until the underlying listener is closed.
func (m *connMux) serve() error {
	defer m.tls.Close()
	defer m.plain.Close()
	var delay time.Duration
	for {
		c, err := m.ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return err
			}
			// Temporary failures such as "too many open files": back off the
			// way net/http does instead of spinning.
			delay = min(max(2*delay, 5*time.Millisecond), time.Second)
			time.Sleep(delay)
			continue
		}
		delay = 0
		go m.route(c)
	}
}

// route peeks at the first byte of c to tell TLS from plain HTTP.
func (m *connMux) route(c net.Conn) {
	c.SetReadDeadline(time.Now().Add(readHeaderTimeout))
	br := bufio.NewReader(c)
	first, err := br.Peek(1)
	c.SetReadDeadline(time.Time{})
	if err != nil {
		c.Close()
		return
	}
	target := m.plain
	if first[0] == tlsHandshake {
		target = m.tls
	}
	target.push(&peekedConn{Conn: c, r: br})
}

// peekedConn is a connection whose first bytes were buffered while routing.
type peekedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *peekedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// chanListener is a net.Listener fed with connections by a connMux.
type chanListener struct {
	addr  net.Addr
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func newChanListener(addr net.Addr) *chanListener {
	return &chanListener{addr: addr, conns: make(chan net.Conn), done: make(chan struct{})}
}

func (l *chanListener) push(c net.Conn) {
	select {
	case l.conns <- c:
	case <-l.done:
		c.Close()
	}
}

func (l *chanListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *chanListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *chanListener) Addr() net.Addr { return l.addr }
