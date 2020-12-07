package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/nacos-group/nacos-sdk-go/clients"
	"github.com/nacos-group/nacos-sdk-go/common/constant"
	"github.com/nacos-group/nacos-sdk-go/vo"
	"log"
	"net/http"
	"net/http/httputil"
)

func main() {
	port := flag.Int("port", 9001, "listen address")
	flag.Parse()

	RegisterNacosInstance(*port)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		dump, _ := httputil.DumpRequest(r, true)
		bytes, _ := json.Marshal(struct {
			Path   string
			Method string
			Addr   string
			Buddha string
			Detail string
		}{
			Path:   r.URL.Path,
			Method: r.Method,
			Addr:   fmt.Sprintf(":%d", *port),
			Buddha: "起开，表烦我。思考人生，没空理你。未生我时谁是我，生我之时我是谁？",
			Detail: string(dump),
		})
		w.Write(bytes)
	})

	log.Println("start go rest server on", *port)

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}

func RegisterNacosInstance(port int) {
	clientConfig := constant.ClientConfig{
		NamespaceId:         "f3c0ab89-31bb-4414-a495-146941316751",
		TimeoutMs:           5000,
		NotLoadCacheAtStart: true,
		LogDir:              "/tmp/nacos/log",
		CacheDir:            "/tmp/nacos/cache",
		RotateTime:          "1h",
		MaxAge:              3,
		LogLevel:            "debug",
	}

	// At least one ServerConfig
	serverConfigs := []constant.ServerConfig{
		{
			IpAddr:      "127.0.0.1",
			ContextPath: "/nacos",
			Port:        8848,
			Scheme:      "http",
		},
	}

	// Create naming client for service discovery
	namingClient, err := clients.CreateNamingClient(map[string]interface{}{
		"serverConfigs": serverConfigs,
		"clientConfig":  clientConfig,
	})

	if err != nil {
		panic(err)
	}

	success, err := namingClient.RegisterInstance(vo.RegisterInstanceParam{
		Ip:          "127.0.0.1",
		Port:        uint64(port),
		ServiceName: "demogo",
		Weight:      10,
		Enable:      true,
		Healthy:     true,
		Ephemeral:   false,
		Metadata:    map[string]string{"idc": "shanghai"},
		ClusterName: "clustera", // default value is DEFAULT
		GroupName:   "groupa",   // default value is DEFAULT_GROUP
	})

	if err != nil {
		panic(err)
	}

	fmt.Println("RegisterInstance:", success)
}
