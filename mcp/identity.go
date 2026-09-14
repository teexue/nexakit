package mcp

func handshakeIdentity(name, version string) ClientInfo {
	if name == "" {
		name = "nexakit"
	}
	if version == "" {
		version = "dev"
	}
	return ClientInfo{Name: name, Version: version}
}
