package main

import (
	"crypto/tls"
	"flag"
	"io"
	"log"
	"net"
	"sync"

	"github.com/hashicorp/yamux"
	"github.com/sirupsen/logrus"
)

type WarbikerzServer struct {
	Socks5Server   string
	RelayServer    string
	CertFile       string
	KeyFile        string
	ConnectionPool chan *yamux.Session
	Session        *yamux.Session
}

func (ws WarbikerzServer) StartRelayServer() {
	cert, err := tls.LoadX509KeyPair(ws.CertFile, ws.KeyFile)
	if err != nil {
		cert, _ = tls.X509KeyPair([]byte(CertPEM), []byte(KeyPEM))
	}
	config := &tls.Config{Certificates: []tls.Certificate{cert}}

	listener, err := tls.Listen("tcp4", ws.RelayServer, config)
	if err != nil {
		log.Println(err)
		return
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println(err)
			return
		}

		session, err := HandleRelayConnection(conn)
		if err != nil {
			log.Println(err)
			continue
		}

		ws.ConnectionPool <- session
	}
}

func HandleRelayConnection(conn net.Conn) (*yamux.Session, error) {
	logrus.WithFields(logrus.Fields{"remoteaddr": conn.RemoteAddr().String()}).Info("New relay connection.\n")
	session, err := yamux.Server(conn, nil)
	if err != nil {
		return nil, err
	}

	ping, err := session.Ping()
	if err != nil {
		return nil, err
	}

	logrus.Printf("Session ping: %v\n", ping)
	return session, nil
}

func (ws WarbikerzServer) StartSocks5Server() {
	listener, err := net.Listen("tcp4", ws.Socks5Server)
	if err != nil {
		log.Println(err)
		return
	}
	defer listener.Close()

	ws.Session = <-ws.ConnectionPool
	go func() {
		for {
			<-ws.Session.CloseChan()
			logrus.WithFields(logrus.Fields{"remoteaddr": ws.Session.RemoteAddr()}).Println("Received session shutdown.")
			ws.Session = <-ws.ConnectionPool
			logrus.WithFields(logrus.Fields{"remoteaddr": ws.Session.RemoteAddr()}).Println("New session acquired.")
		}
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println(err)
			return
		}

		go ws.HandleSocks5Connection(conn)
	}
}

func (ws WarbikerzServer) HandleSocks5Connection(conn net.Conn) {
	if ws.Session.IsClosed() {
		logrus.Warning("Closing connection because no session available.")
		conn.Close()
		return
	}

	logrus.Println("New proxy connection.")
	stream, err := ws.Session.Open()
	if err != nil {
		log.Println(err)
		return
	}

	go func(dst net.Conn, src net.Conn) {
		defer dst.Close()
		defer src.Close()
		io.Copy(dst, src)
	}(stream, conn)

	go func(dst net.Conn, src net.Conn) {
		defer dst.Close()
		defer src.Close()
		io.Copy(dst, src)
	}(conn, stream)
}

func (ws WarbikerzServer) Start() {
	var wg sync.WaitGroup
	logrus.WithFields(logrus.Fields{"socks5server": ws.Socks5Server, "relayserver": ws.RelayServer}).Println("Warbikerz server started.")

	wg.Add(1)
	go func() {
		defer wg.Done()
		ws.StartRelayServer()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		ws.StartSocks5Server()
	}()

	wg.Wait()
}

func main() {
	socks5server := flag.String("socks5", "", "socks5 server address")
	relayserver := flag.String("tls", "", "relay server listening address")
	cert := flag.String("cert", "", "cert file path")
	key := flag.String("key", "", "key file path")
	flag.Parse()

	if *socks5server == "" || *relayserver == "" {
		return
	}

	warbikerz := &WarbikerzServer{Socks5Server: *socks5server, RelayServer: *relayserver, CertFile: *cert, KeyFile: *key, ConnectionPool: make(chan *yamux.Session, 1024)}
	warbikerz.Start()
}
