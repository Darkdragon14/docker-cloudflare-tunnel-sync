package model

// AccessAppSpec describes the desired Access application state.
type AccessAppSpec struct {
	ID       string
	Name     string
	Domain   string
	Policies []AccessPolicySpec
	Tags     []string
	TagsSet  bool
	Source   SourceRef
}

// AccessPolicySpec describes the desired Access policy state.
type AccessPolicySpec struct {
	ID            string
	Name          string
	Action        string
	IncludeEmails []string
	IncludeIPs    []string
	Managed       bool
}
