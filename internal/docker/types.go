package docker

// ContainerInfo contains the label metadata needed for reconciliation.
type ContainerInfo struct {
	ID     string
	Name   string
	Labels map[string]string
	// Ports lists ports exposed by the container (Dockerfile EXPOSE, compose expose/ports).
	Ports []PortInfo
	// Networks maps attached network names to the container IP address on that network.
	Networks map[string]string
}

// PortInfo describes a single exposed container port.
type PortInfo struct {
	PrivatePort uint16
	PublicPort  uint16
	Type        string
}

// ContainerEvent is a container lifecycle event reported by the Docker daemon.
type ContainerEvent struct {
	ID     string
	Name   string
	Action string
}
