package specgen

import (
	"strings"

	"github.com/pkg/errors"
)

type NamespaceMode string

const (
	// Default indicates the spec generator should determine
	// a sane default
	Default NamespaceMode = "default"
	// Host means the the namespace is derived from
	// the host
	Host NamespaceMode = "host"
	// Path is the path to a namespace
	Path NamespaceMode = "path"
	// FromContainer means namespace is derived from a
	// different container
	FromContainer NamespaceMode = "container"
	// FromPod indicates the namespace is derived from a pod
	FromPod NamespaceMode = "pod"
	// Private indicates the namespace is private
	Private NamespaceMode = "private"
	// NoNetwork indicates no network namespace should
	// be joined.  loopback should still exists
	NoNetwork NamespaceMode = "none"
	// Bridge indicates that a CNI network stack
	// should be used
	Bridge NamespaceMode = "bridge"
	// Slirp indicates that a slirp4netns network stack should
	// be used
	Slirp NamespaceMode = "slirp4netns"
)

// Namespace describes the namespace
type Namespace struct {
	NSMode NamespaceMode `json:"nsmode,omitempty"`
	Value  string        `json:"string,omitempty"`
}

// IsDefault returns whether the namespace is set to the default setting (which
// also includes the empty string).
func (n *Namespace) IsDefault() bool {
	return n.NSMode == Default || n.NSMode == ""
}

// IsHost returns a bool if the namespace is host based
func (n *Namespace) IsHost() bool {
	return n.NSMode == Host
}

// IsPath indicates via bool if the namespace is based on a path
func (n *Namespace) IsPath() bool {
	return n.NSMode == Path
}

// IsContainer indicates via bool if the namespace is based on a container
func (n *Namespace) IsContainer() bool {
	return n.NSMode == FromContainer
}

// IsPod indicates via bool if the namespace is based on a pod
func (n *Namespace) IsPod() bool {
	return n.NSMode == FromPod
}

// IsPrivate indicates the namespace is private
func (n *Namespace) IsPrivate() bool {
	return n.NSMode == Private
}

func validateNetNS(n *Namespace) error {
	if n == nil {
		return nil
	}
	switch n.NSMode {
	case "", Default, Host, Path, FromContainer, FromPod, Private, NoNetwork, Bridge, Slirp:
		break
	default:
		return errors.Errorf("invalid network %q", n.NSMode)
	}

	// Path and From Container MUST have a string value set
	if n.NSMode == Path || n.NSMode == FromContainer {
		if len(n.Value) < 1 {
			return errors.Errorf("namespace mode %s requires a value", n.NSMode)
		}
	} else {
		// All others must NOT set a string value
		if len(n.Value) > 0 {
			return errors.Errorf("namespace value %s cannot be provided with namespace mode %s", n.Value, n.NSMode)
		}
	}

	return nil
}

// Validate perform simple validation on the namespace to make sure it is not
// invalid from the get-go
func (n *Namespace) validate() error {
	if n == nil {
		return nil
	}
	switch n.NSMode {
	case "", Default, Host, Path, FromContainer, FromPod, Private:
		// Valid, do nothing
	case NoNetwork, Bridge, Slirp:
		return errors.Errorf("cannot use network modes with non-network namespace")
	default:
		return errors.Errorf("invalid namespace type %s specified", n.NSMode)
	}

	// Path and From Container MUST have a string value set
	if n.NSMode == Path || n.NSMode == FromContainer {
		if len(n.Value) < 1 {
			return errors.Errorf("namespace mode %s requires a value", n.NSMode)
		}
	} else {
		// All others must NOT set a string value
		if len(n.Value) > 0 {
			return errors.Errorf("namespace value %s cannot be provided with namespace mode %s", n.Value, n.NSMode)
		}
	}
	return nil
}

// ParseNamespace parses a namespace in string form.
// This is not intended for the network namespace, which has a separate
// function.
func ParseNamespace(ns string) (Namespace, error) {
	toReturn := Namespace{}
	switch {
	case ns == "host":
		toReturn.NSMode = Host
	case ns == "private":
		toReturn.NSMode = Private
	case strings.HasPrefix(ns, "ns:"):
		split := strings.SplitN(ns, ":", 2)
		if len(split) != 2 {
			return toReturn, errors.Errorf("must provide a path to a namespace when specifying ns:")
		}
		toReturn.NSMode = Path
		toReturn.Value = split[1]
	case strings.HasPrefix(ns, "container:"):
		split := strings.SplitN(ns, ":", 2)
		if len(split) != 2 {
			return toReturn, errors.Errorf("must provide name or ID or a container when specifying container:")
		}
		toReturn.NSMode = FromContainer
		toReturn.Value = split[1]
	default:
		return toReturn, errors.Errorf("unrecognized namespace mode %s passed", ns)
	}

	return toReturn, nil
}

// ParseNetworkNamespace parses a network namespace specification in string
// form.
// Returns a namespace and (optionally) a list of CNI networks to join.
func ParseNetworkNamespace(ns string) (Namespace, []string, error) {
	toReturn := Namespace{}
	var cniNetworks []string
	switch {
	case ns == "bridge":
		toReturn.NSMode = Bridge
	case ns == "none":
		toReturn.NSMode = NoNetwork
	case ns == "host":
		toReturn.NSMode = Host
	case ns == "private":
		toReturn.NSMode = Private
	case strings.HasPrefix(ns, "ns:"):
		split := strings.SplitN(ns, ":", 2)
		if len(split) != 2 {
			return toReturn, nil, errors.Errorf("must provide a path to a namespace when specifying ns:")
		}
		toReturn.NSMode = Path
		toReturn.Value = split[1]
	case strings.HasPrefix(ns, "container:"):
		split := strings.SplitN(ns, ":", 2)
		if len(split) != 2 {
			return toReturn, nil, errors.Errorf("must provide name or ID or a container when specifying container:")
		}
		toReturn.NSMode = FromContainer
		toReturn.Value = split[1]
	default:
		// Assume we have been given a list of CNI networks.
		// Which only works in bridge mode, so set that.
		cniNetworks = strings.Split(ns, ",")
		toReturn.NSMode = Bridge
	}

	return toReturn, cniNetworks, nil
}
