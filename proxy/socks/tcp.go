package socks

import (
	"fmt"
	"io"
	"net"
	"sync"

	"golang.org/x/net/proxy"

	"github.com/eycorsican/go-tun2socks/common/log"
	"github.com/eycorsican/go-tun2socks/core"
)

type tcpHandler struct {
	sync.Mutex

	proxyHost string
	proxyPort uint16
	Uname     string
	Pwd       string
}

func NewTCPHandler(proxyHost string, proxyPort uint16, auths ...string) core.TCPConnHandler {
	th := &tcpHandler{
		proxyHost: proxyHost,
		proxyPort: proxyPort,
	}
	if len(auths) > 0 {
		th.Uname = auths[0]
		th.Pwd = auths[1]
	}
	return th
}

type direction byte

const (
	dirUplink direction = iota
	dirDownlink
)

type duplexConn interface {
	net.Conn
	CloseRead() error
	CloseWrite() error
}

func (h *tcpHandler) relay(lhs, rhs net.Conn) {
	upCh := make(chan struct{})

	cls := func(dir direction, interrupt bool) {
		lhsDConn, lhsOk := lhs.(duplexConn)
		rhsDConn, rhsOk := rhs.(duplexConn)
		if !interrupt && lhsOk && rhsOk {
			switch dir {
			case dirUplink:
				lhsDConn.CloseRead()
				rhsDConn.CloseWrite()
			case dirDownlink:
				lhsDConn.CloseWrite()
				rhsDConn.CloseRead()
			default:
				panic("unexpected direction")
			}
		} else {
			lhs.Close()
			rhs.Close()
		}
	}

	// Uplink
	go func() {
		var err error
		_, err = io.Copy(rhs, lhs)
		if err != nil {
			cls(dirUplink, true) // interrupt the conn if the error is not nil (not EOF)
		} else {
			cls(dirUplink, false) // half close uplink direction of the TCP conn if possible
		}
		upCh <- struct{}{}
	}()

	// Downlink
	var err error
	_, err = io.Copy(lhs, rhs)
	if err != nil {
		cls(dirDownlink, true)
	} else {
		cls(dirDownlink, false)
	}

	<-upCh // Wait for uplink done.
}

func (h *tcpHandler) Handle(conn net.Conn, target *net.TCPAddr) error {
	var auth *proxy.Auth
	if h.Uname != "" && h.Pwd != "" {
		auth = &proxy.Auth{
			User:     h.Uname,
			Password: h.Pwd,
		}
	}
	fmt.Println("执行tcphandle")
	fmt.Println("auth的值", *auth)
	dialer, err := proxy.SOCKS5("tcp", core.ParseTCPAddr(h.proxyHost, h.proxyPort).String(), auth, nil)
	if err != nil {
		return err
	}

	c, err := dialer.Dial(target.Network(), target.String())
	if err != nil {
		return err
	}

	go h.relay(conn, c)

	log.Infof("new proxy connection to %v", target)

	return nil
}
