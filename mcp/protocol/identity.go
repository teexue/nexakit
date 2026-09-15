package protocol

// HandshakeIdentity resolves the caller-supplied handshake name/version,
// falling back to the kit defaults when either is empty.
func HandshakeIdentity(name, version string) ClientInfo {
	if name == "" {
		name = "nexakit"
	}
	if version == "" {
		version = "dev"
	}
	return ClientInfo{Name: name, Version: version}
}
