package git

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	gitbackend "github.com/go-git/go-git/v6/backend/git"
	"github.com/go-git/go-git/v6/plumbing/format/pktline"
	"github.com/go-git/go-git/v6/plumbing/protocol/packp"
	"github.com/go-git/go-git/v6/plumbing/transport"
	"github.com/go-git/go-git/v6/utils/ioutil"
)

// DefaultAddr is the default address to listen on for Git protocol server.
const DefaultAddr = ":9418"

// ErrServerClosed indicates that the server has been closed.
var ErrServerClosed = errors.New("server closed")

// DefaultBackend is the default global Git transport server handler.
var DefaultBackend = gitbackend.NewBackend(nil)

// ServerContextKey is the context key used to store the server in the context.
var ServerContextKey = &contextKey{"git-server"}

// Handler is the interface that handles TCP requests for the Git protocol.
type Handler interface {
	// ServeTCP handles a TCP connection for the Git protocol.
	ServeTCP(ctx context.Context, c io.ReadWriteCloser, req *packp.GitProtoRequest)
}

// HandlerFunc is a function that implements the Handler interface.
type HandlerFunc func(ctx context.Context, c io.ReadWriteCloser, req *packp.GitProtoRequest)

// ServeTCP implements the Handler interface.
func (f HandlerFunc) ServeTCP(ctx context.Context, c io.ReadWriteCloser, req *packp.GitProtoRequest) {
	f(ctx, c, req)
}

// Server is a TCP server that handles Git protocol requests.
type Server struct {
	// Addr is the address to listen on. If empty, it defaults to ":9418".
	Addr string

	// Handler is the handler for Git protocol requests. It uses
	// [DefaultHandler] when nil.
	Handler Handler

	// ErrorLog is the logger used to log errors. When nil, it won't log
	// errors.
	ErrorLog *log.Logger

	// BaseContext optionally specifies a function to create a base context for
	// the server listeners. If nil, [context.Background] will be used.
	// The provided listener is the specific listener that is about to start
	// accepting connections.
	BaseContext func(net.Listener) context.Context

	// ConnContext optionally specifies a function to create a context for each
	// connection. If nil, the context will be derived from the server's base
	// context.
	ConnContext func(context.Context, net.Conn) context.Context

	inShutdown    atomic.Bool // true when server is in shutdown
	mu            sync.Mutex
	listeners     map[*net.Listener]struct{}
	listenerGroup sync.WaitGroup
	activeConn    map[*conn]struct{} // active connections being served
}

// shutdownPollIntervalMax is the maximum interval for polling
// idle connections during shutdown.
const shutdownPollIntervalMax = 500 * time.Millisecond

// Shutdown gracefully shuts down the server, waiting for all active
// connections to finish.
func (s *Server) Shutdown(ctx context.Context) error {
	s.inShutdown.Store(true)

	s.mu.Lock()
	lnerr := s.closeListenersLocked()
	s.mu.Unlock()
	s.listenerGroup.Wait()

	pollIntervalBase := time.Millisecond
	nextPollInterval := func() time.Duration {
		// Add 10% jitter.
		interval := pollIntervalBase + time.Duration(rand.Intn(int(pollIntervalBase/10)))
		// Double and clamp for next time.
		pollIntervalBase *= 2
		if pollIntervalBase > shutdownPollIntervalMax {
			pollIntervalBase = shutdownPollIntervalMax
		}

		return interval
	}

	timer := time.NewTimer(nextPollInterval())

	for {
		if s.closeIdleConns() {
			return lnerr
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			timer.Reset(nextPollInterval())
		}
	}
}

// Close immediately closes the server and all active connections. It returns
// any error returned from closing the underlying listeners.
func (s *Server) Close() error {
	s.inShutdown.Store(true)

	s.mu.Lock()
	defer s.mu.Unlock()

	err := s.closeListenersLocked()

	// We need to unlock the mutex while waiting for listenersGroup.
	s.mu.Unlock()
	s.listenerGroup.Wait()
	s.mu.Lock()

	for c := range s.activeConn {
		c.Close()
		delete(s.activeConn, c)
	}

	return err
}

// ListenAndServe listens on the TCP network address and serves Git
// protocol requests using the provided handler.
func (s *Server) ListenAndServe() error {
	if s.shuttingDown() {
		return ErrServerClosed
	}

	addr := s.Addr
	if addr == "" {
		addr = DefaultAddr // Default Git protocol port
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	return s.Serve(ln)
}

// Serve starts the server and listens for incoming connections on the given
// listener.
func (s *Server) Serve(ln net.Listener) error {
	origLn := ln

	l := &onceCloseListener{Listener: ln}
	defer l.Close()

	if !s.trackListener(&l.Listener, true) {
		return ErrServerClosed
	}
	defer s.trackListener(&l.Listener, false)

	baseCtx := context.Background()
	if s.BaseContext != nil {
		baseCtx = s.BaseContext(origLn)
		if baseCtx == nil {
			panic("git: BaseContext returned nil context")
		}
	}

	var tempDelay time.Duration // how long to sleep on accept failure

	ctx := context.WithValue(baseCtx, ServerContextKey, s)

	for {
		rw, err := l.Accept()
		if err != nil {
			if s.shuttingDown() {
				return ErrServerClosed
			}

			var ne net.Error
			if errors.As(err, &ne) {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}

				tempDelay = min(tempDelay, 1*time.Second)

				s.logf("git: Accept error: %v; retrying in %v", err, tempDelay)
				time.Sleep(tempDelay)

				continue
			}

			return err
		}

		connCtx := ctx
		if cc := s.ConnContext; cc != nil {
			connCtx = cc(ctx, rw)
			if connCtx == nil {
				panic("git: ConnContext returned nil context")
			}
		}

		tempDelay = 0
		c := s.newConn(rw)
		s.trackConn(c, true)

		go c.serve(connCtx)
	}
}

func (s *Server) shuttingDown() bool {
	return s.inShutdown.Load()
}

func (s *Server) closeListenersLocked() error {
	var err error
	for ln := range s.listeners {
		if cerr := (*ln).Close(); cerr != nil && err == nil {
			err = cerr
		}
	}

	return err
}

// handler delegates to either the server's Handler or the DefaultBackend.
func (s *Server) handler(ctx context.Context, c net.Conn, req *packp.GitProtoRequest) {
	if s.Handler != nil {
		s.Handler.ServeTCP(ctx, c, req)
	} else {
		DefaultBackend.ServeTCP(ctx, c, req)
	}
}

// trackListener adds or removes a net.Listener to the set of tracked
// listeners.
//
// We store a pointer to interface in the map set, in case the
// net.Listener is not comparable. This is safe because we only call
// trackListener via Serve and can track+defer untrack the same
// pointer to local variable there. We never need to compare a
// Listener from another caller.
//
// It reports whether the server is still up (not Shutdown or Closed).
func (s *Server) trackListener(ln *net.Listener, add bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listeners == nil {
		s.listeners = make(map[*net.Listener]struct{})
	}

	if add {
		if s.shuttingDown() {
			return false
		}

		s.listeners[ln] = struct{}{}
		s.listenerGroup.Add(1)
	} else {
		delete(s.listeners, ln)
		s.listenerGroup.Done()
	}

	return true
}

// closeIdleConns closes all idle connections. It returns true only if no new
// connection was found.
func (s *Server) closeIdleConns() bool {
	idle := true

	for c := range s.activeConn {
		unixSec := c.unixSec.Load()
		if unixSec == 0 {
			// New connection, skip it.
			idle = false

			continue
		}

		c.Close()
		delete(s.activeConn, c)
	}

	return idle
}

func (s *Server) logf(format string, args ...any) {
	if s.ErrorLog != nil {
		s.ErrorLog.Printf(format, args...)
	}
}

func (s *Server) trackConn(c *conn, add bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c.unixSec.Store(uint64(time.Now().Unix()))

	if s.activeConn == nil {
		s.activeConn = make(map[*conn]struct{})
	}

	if add {
		s.activeConn[c] = struct{}{}
	} else {
		delete(s.activeConn, c)
	}
}

// conn represents a server connection that is being handled.
type conn struct {
	// Conn is the underlying net.Conn that is being used to read and write Git
	// protocol messages.
	net.Conn

	// unix timestamp in seconds when the connection was established
	unixSec atomic.Uint64
	// s the server that is handling this connection.
	s *Server
}

// newConn creates a new conn instance with the given net.Conn.
func (s *Server) newConn(rwc net.Conn) *conn {
	return &conn{
		s:    s,
		Conn: rwc,
	}
}

// serve serves a new connection.
func (c *conn) serve(ctx context.Context) {
	defer func() {
		if err := recover(); err != nil {
			c.s.logf("git: panic serving connection: %v", err)

			if cerr := c.Close(); cerr != nil {
				c.s.logf("git: error closing connection: %v", cerr)
			}
		}
	}()

	r := ioutil.NewContextReadCloser(ctx, c)

	var req packp.GitProtoRequest
	if err := req.Decode(r); err != nil {
		c.s.logf("git: error decoding request: %v", err)

		if rErr := renderError(c, fmt.Errorf("error decoding request: %w", transport.ErrInvalidRequest)); rErr != nil {
			c.s.logf("git: error writing error response: %v", rErr)
		}

		return
	}

	c.s.handler(ctx, c.Conn, &req)
}

// onceCloseListener wraps a net.Listener, protecting it from
// multiple Close calls.
type onceCloseListener struct {
	net.Listener

	once     sync.Once
	closeErr error
}

func (oc *onceCloseListener) Close() error {
	oc.once.Do(oc.close)

	return oc.closeErr
}

func (oc *onceCloseListener) close() { oc.closeErr = oc.Listener.Close() }

// contextKey is a value for use with context.WithValue. It's used as
// a pointer so it fits in an interface{} without allocation.
type contextKey struct {
	name string
}

func renderError(rw io.WriteCloser, err error) error {
	if _, err := pktline.WriteError(rw, err); err != nil {
		rw.Close()

		return err
	}

	return rw.Close()
}
