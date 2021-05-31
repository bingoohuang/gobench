package main

import (
	"fmt"
	"github.com/bingoohuang/gg/pkg/flagparse"
	"github.com/bingoohuang/gg/pkg/rest"
	"github.com/bingoohuang/gg/pkg/ss"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Config struct {
	Addr   string `val:"127.0.0.1:5003" usage:"connecting address like 127.0.0.1:5003"`
	Input  string `usage:"http request filename or direct input content"`
	Server bool   `usage:"start as a server"`
}

func main() {
	c := &Config{}
	flagparse.Parse(c)

	if c.Server {
		c.server()
	} else {
		c.client()
	}
}

func (c *Config) client() {
	conn, err := net.Dial("tcp", c.Addr)
	if err != nil {
		panic(err)
	}
	defer conn.Close() // 关闭TCP连接

	data, err := c.readInput()
	if err != nil {
		panic(err)
	}

	if err := c.send(conn, data); err != nil {
		panic(err)
	}
}

func (c *Config) send(conn net.Conn, data []byte) error {
	//data := "OPTIONS * HTTP/1.1\r\nHost: localhost:5003\r\nContent-Length: 18\r\nContent-Type: application/json\r\n\r\n{\"name\": \"bingoo\"}"
	s := strings.TrimSpace(strings.NewReplacer(`\r\n`, "\r\n", `\n`, "\r\n", "\r\n", "\r\n", "\n", "\r\n").Replace(string(data)))
	if v := FindContentLength(s); v == 0 {
		s += "\r\n\r\n"
	}

	fmt.Printf("Request: %s\n", ss.Jsonify(s))

	if _, err := conn.Write([]byte(s)); err != nil { // 发送数据
		return err
	}

	buf := [512]byte{}
	n, err := conn.Read(buf[:])
	if err != nil {
		return err
	}
	fmt.Println(string(buf[:n]))

	return nil
}

func (c *Config) readInput() ([]byte, error) {
	data, err := ioutil.ReadFile(c.Input)
	if err == nil {
		return data, nil
	}

	if os.IsNotExist(err) {
		return []byte(c.Input), nil
	}

	return data, err
}

func (c *Config) server() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		rest.LogRequest(r, "all")
	})

	panic(http.ListenAndServe(c.Addr, mux))
}

var re = regexp.MustCompile(`(?i)Content-Length:\s*(\d+)`)

func FindContentLength(s string) int {
	contentLength := 0
	if subs := re.FindStringSubmatch(s); len(subs) > 0 {
		contentLength, _ = strconv.Atoi(subs[1])
	}
	return contentLength
}
