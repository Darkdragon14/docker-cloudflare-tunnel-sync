package docker

// ContainerInfo contains the label metadata needed for reconciliation.
type ContainerInfo struct {
	ID     string
	Name   string
	Labels map[string]string
}
