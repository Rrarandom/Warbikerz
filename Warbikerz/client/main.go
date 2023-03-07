package main

import (
	"Warbikerz/socks5"
	"Warbikerz/yamux"
	"crypto/tls"
	"flag"
	"time"
)

func StartWarbikerz(server string, username string, password string) error {
	config := &tls.Config{InsecureSkipVerify: true}
	conn, err := tls.Dial("tcp", server, config)
	if err != nil {
		return err
	}

	var conf *socks5.Config
	if username == "" || password == "" {
		conf = &socks5.Config{}
	} else {
		creds := socks5.StaticCredentials{username: password}
		cator := socks5.UserPassAuthenticator{Credentials: creds}
		conf = &socks5.Config{AuthMethods: []socks5.Authenticator{cator}}
	}

	serv, err := socks5.New(conf)
	if err != nil {
		return err
	}

	session, err := yamux.Client(conn, nil)
	if err != nil {
		return err
	}

	for {
		stream, err := session.Accept()
		if err != nil {
			return err
		}
		go serv.ServeConn(stream)
	}
}

func main() {
	server := flag.String("s", "", "ip:port")
	username := flag.String("u", "", "socks5 username")
	password := flag.String("p", "", "socks5 password")
	flag.Parse()

	if *server == "" {
		return
	}

	for {
		StartWarbikerz(*server, *username, *password)
		time.Sleep(10 * time.Second)
	}
}
