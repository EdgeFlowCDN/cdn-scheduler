package dns

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/miekg/dns"

	"github.com/EdgeFlowCDN/cdn-scheduler/config"
	"github.com/EdgeFlowCDN/cdn-scheduler/scheduler"
)

// Server is a DNS server that responds with the best edge node IP.
type Server struct {
	scheduler  *scheduler.Scheduler
	domain     string
	ttl        uint32
	addr       string
	udpServer  *dns.Server
	tcpServer  *dns.Server
}

// NewServer creates a new DNS scheduler server.
func NewServer(sched *scheduler.Scheduler, cfg config.DNSConfig) *Server {
	domain := cfg.Domain
	if !strings.HasSuffix(domain, ".") {
		domain += "."
	}
	return &Server{
		scheduler: sched,
		domain:    domain,
		ttl:       cfg.TTL,
		addr:      cfg.Listen,
	}
}

// ListenAndServe starts the DNS server on UDP and TCP.
func (s *Server) ListenAndServe() error {
	mux := dns.NewServeMux()
	mux.HandleFunc(s.domain, s.handleDNS)
	mux.HandleFunc(".", s.handleDefault)

	errCh := make(chan error, 2)

	s.udpServer = &dns.Server{Addr: s.addr, Net: "udp", Handler: mux}
	s.tcpServer = &dns.Server{Addr: s.addr, Net: "tcp", Handler: mux}

	// UDP
	go func() {
		log.Printf("[dns] listening on %s (UDP)", s.addr)
		errCh <- s.udpServer.ListenAndServe()
	}()

	// TCP
	go func() {
		log.Printf("[dns] listening on %s (TCP)", s.addr)
		errCh <- s.tcpServer.ListenAndServe()
	}()

	return <-errCh
}

func (s *Server) handleDNS(w dns.ResponseWriter, r *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true

	for _, q := range r.Question {
		if q.Qtype != dns.TypeA {
			continue
		}

		clientIP := extractClientIP(w, r)
		node := s.scheduler.SelectNode(clientIP)
		if node == nil {
			log.Printf("[dns] no available node for client %s", clientIP)
			continue
		}

		ip := net.ParseIP(node.IP)
		if ip == nil {
			continue
		}

		rr := &dns.A{
			Hdr: dns.RR_Header{
				Name:   q.Name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    s.ttl,
			},
			A: ip.To4(),
		}
		msg.Answer = append(msg.Answer, rr)
		log.Printf("[dns] %s -> %s (client: %s, node: %s)", q.Name, node.IP, clientIP, node.Name)
	}

	w.WriteMsg(msg)
}

func (s *Server) handleDefault(w dns.ResponseWriter, r *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Rcode = dns.RcodeNameError
	w.WriteMsg(msg)
}

// extractClientIP gets the client IP, checking EDNS Client Subnet first.
func extractClientIP(w dns.ResponseWriter, r *dns.Msg) string {
	// Check EDNS Client Subnet (ECS)
	if opt := r.IsEdns0(); opt != nil {
		for _, o := range opt.Option {
			if ecs, ok := o.(*dns.EDNS0_SUBNET); ok {
				return ecs.Address.String()
			}
		}
	}

	// Fall back to resolver IP
	addr := w.RemoteAddr()
	if addr == nil {
		return "0.0.0.0"
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}

// Shutdown gracefully shuts down the DNS server.
func (s *Server) Shutdown() {
	if s.udpServer != nil {
		s.udpServer.Shutdown()
	}
	if s.tcpServer != nil {
		s.tcpServer.Shutdown()
	}
}

// HTTPRedirectServer provides HTTP 302 scheduling.
type HTTPRedirectServer struct {
	scheduler *scheduler.Scheduler
	addr      string
	server    *http.Server
}

// NewHTTPRedirectServer creates an HTTP 302 redirect scheduler.
func NewHTTPRedirectServer(sched *scheduler.Scheduler, addr string) *HTTPRedirectServer {
	return &HTTPRedirectServer{scheduler: sched, addr: addr}
}

// ListenAndServe starts the HTTP redirect server.
func (s *HTTPRedirectServer) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRedirect)
	s.server = &http.Server{Addr: s.addr, Handler: mux}
	log.Printf("[http] redirect server listening on %s", s.addr)
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP redirect server.
func (s *HTTPRedirectServer) Shutdown(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

func (s *HTTPRedirectServer) handleRedirect(w http.ResponseWriter, r *http.Request) {
	clientIP := r.Header.Get("X-Real-IP")
	if clientIP == "" {
		clientIP, _, _ = net.SplitHostPort(r.RemoteAddr)
	}

	node := s.scheduler.SelectNode(clientIP)
	if node == nil {
		http.Error(w, "no available node", http.StatusServiceUnavailable)
		return
	}

	redirectURL := fmt.Sprintf("http://%s%s", node.IP, r.URL.RequestURI())
	http.Redirect(w, r, redirectURL, http.StatusFound)
}
