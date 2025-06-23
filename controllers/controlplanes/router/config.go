package router

import (
	"strconv"
	"strings"
)

func GetConfig(namespace string) string {
	// Default values for parameters
	replacer := strings.NewReplacer(
		"<MESSAGE_PORT>", strconv.Itoa(MessagePort),
		"<HTTP_PORT>", strconv.Itoa(HTTPPort),
		"<INTERIOR_PORT>", strconv.Itoa(InteriorPort),
		"<EDGE_PORT>", strconv.Itoa(EdgePort),
		"<NAMESPACE>", namespace,
	)

	return replacer.Replace(rawRouterConfig)
}

const (
	MessagePort  = 5671
	HTTPPort     = 9090
	InteriorPort = 55671
	EdgePort     = 45671
)

const rawRouterConfig = `
[
    [
        "router",
        {
            "id": "default-router",
            "mode": "interior",
            "helloMaxAgeSeconds": "3",
            "metadata": "{\"id\":\"default-router\",\"version\":\"pot\",\"platform\":\"kubernetes\",\"pot-config\":\"1.0.0\"}"
        }
    ],
    [
        "site",
        {
            "name": "default-router",
            "platform": "kubernetes",
            "namespace": "<NAMESPACE>",
            "version": "pot"
        }
    ],
    [
        "sslProfile",
        {
            "name": "pot-router-site-server",
            "certFile": "/etc/skupper-router-certs/pot-router-site-server/tls.crt",
            "privateKeyFile": "/etc/skupper-router-certs/pot-router-site-server/tls.key",
            "caCertFile": "/etc/skupper-router-certs/pot-router-site-server/ca.crt"
        }
    ],
    [
        "sslProfile",
        {
            "name": "pot-router-local-server",
            "certFile": "/etc/skupper-router-certs/pot-router-local-server/tls.crt",
            "privateKeyFile": "/etc/skupper-router-certs/pot-router-local-server/tls.key",
            "caCertFile": "/etc/skupper-router-certs/pot-router-local-server/ca.crt"
        }
    ],
    [
        "listener",
        {
            "name": "pot-router-edge",
            "role": "edge",
            "port": <EDGE_PORT>,
            "sslProfile": "pot-router-site-server",
            "saslMechanisms": "EXTERNAL",
            "authenticatePeer": true
        }
    ],
    [
        "listener",
        {
            "name": "amqp",
            "host": "localhost",
            "port": 5672
        }
    ],
    [
        "listener",
        {
            "name": "amqps",
            "port": <MESSAGE_PORT>,
            "sslProfile": "pot-router-local-server",
            "saslMechanisms": "EXTERNAL",
            "authenticatePeer": true
        }
    ],
    [
        "listener",
        {
            "name": "@9090",
            "role": "normal",
            "port": <HTTP_PORT>,
            "http": true,
            "httpRootDir": "disabled",
            "healthz": true,
            "metrics": true
        }
    ],
    [
        "listener",
        {
            "name": "pot-router-inter-router",
            "role": "inter-router",
            "port": <INTERIOR_PORT>,
            "sslProfile": "pot-router-site-server",
            "saslMechanisms": "EXTERNAL",
            "authenticatePeer": true
        }
    ],
    [
        "address",
        {
            "prefix": "mc",
            "distribution": "multicast"
        }
    ],
    [
        "log",
        {
            "module": "ROUTER_CORE",
            "enable": "error+"
        }
    ]
]
`
