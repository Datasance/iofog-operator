package router

import (
	"strconv"
	"strings"
)

func GetConfig(requireSsl, saslMechanisms, authenticatePeer string, secretWithCa *bool) string {
	// Default values for parameters
	if requireSsl == "" {
		requireSsl = "no"
	}
	if saslMechanisms == "" {
		saslMechanisms = "ANONYMOUS"
	}
	if authenticatePeer == "" {
		authenticatePeer = "no"
	}

	// Determine caFile based on secretWithCa
	caFile := "ca.crt" // Default value
	if secretWithCa != nil && !*secretWithCa {
		caFile = "tls.crt"
	}

	replacer := strings.NewReplacer(
		"<MESSAGE_PORT>", strconv.Itoa(MessagePort),
		"<HTTP_PORT>", strconv.Itoa(HTTPPort),
		"<INTERIOR_PORT>", strconv.Itoa(InteriorPort),
		"<EDGE_PORT>", strconv.Itoa(EdgePort),
		"<SASL_MECHANISMS>", saslMechanisms,
		"<AUTHENTICATE_PEER>", authenticatePeer,
		"<REQUIRE_SSL>", requireSsl,
		"<HAS_CA>", caFile,
	)

	return replacer.Replace(rawRouterConfig)
}

const (
	MessagePort  = 5672
	HTTPPort     = 9090
	InteriorPort = 55672
	EdgePort     = 45672
)

const rawRouterConfig = `
router {
    mode: interior
    id: default-router
    saslConfigDir: /etc/sasl2/
}

listener {
    host: 0.0.0.0
    port: <MESSAGE_PORT>
    role: normal
    sslProfile: router-amqps
    requireSsl: <REQUIRE_SSL>
}

sslProfile {
    name: router-amqps
    certFile: /etc/skupper-router/qpid-dispatch-certs/router-amqps/tls.crt
    privateKeyFile: /etc/skupper-router/qpid-dispatch-certs/router-amqps/tls.key
    caCertFile: /etc/skupper-router/qpid-dispatch-certs/router-amqps/<HAS_CA>
}

listener {
    host: 0.0.0.0
    port: <HTTP_PORT>
    role: normal
    http: true
    httpRootDir: disabled
    websockets: false
    healthz: true
    metrics: true
}

sslProfile {
    name: router-internal
    certFile: /etc/skupper-router/qpid-dispatch-certs/router-internal/tls.crt
    privateKeyFile: /etc/skupper-router/qpid-dispatch-certs/router-internal/tls.key
    caCertFile: /etc/skupper-router/qpid-dispatch-certs/router-internal/<HAS_CA>
}

listener {
    role: inter-router
    host: 0.0.0.0
    port: <INTERIOR_PORT>
    saslMechanisms: <SASL_MECHANISMS>
    authenticatePeer: <AUTHENTICATE_PEER>
    sslProfile: router-internal
    requireSsl: <REQUIRE_SSL>
}

listener {
    role: edge
    host: 0.0.0.0
    port: <EDGE_PORT>
    saslMechanisms: <SASL_MECHANISMS>
    authenticatePeer: <AUTHENTICATE_PEER>
    sslProfile: router-internal
    requireSsl: <REQUIRE_SSL>
}

`
