// Package preflight probes a candidate Talos node before the install starts.
//
// Milestone 1 of the install (docs/installer-contract.md): the node is booted
// from the Talos ISO into maintenance mode. A reachable kube-apiserver means
// the node is already installed, so the engine warns and offers to resume
// instead of re-keying it.
package preflight

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"
)

// NodeState is what the probe found.
type NodeState string

const (
	Unreachable NodeState = "unreachable"
	Maintenance NodeState = "maintenance"
	Installed   NodeState = "installed"
)

// Default Talos/Kubernetes ports.
const (
	APIPort = 50000
	K8sPort = 6443
)

// Dial reports whether a TCP connection to addr succeeds within timeout.
func Dial(addr string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return err
	}
	return conn.Close()
}

// Check probes ip on the default Talos (:50000) and Kubernetes (:6443) ports.
func Check(ip string, timeout time.Duration) (NodeState, error) {
	return check(ip, APIPort, K8sPort, timeout)
}

func check(ip string, apiPort, k8sPort int, timeout time.Duration) (NodeState, error) {
	if ip == "" {
		return Unreachable, errors.New("empty node IP")
	}
	if err := Dial(net.JoinHostPort(ip, strconv.Itoa(k8sPort)), timeout); err == nil {
		return Installed, nil
	}
	if err := Dial(net.JoinHostPort(ip, strconv.Itoa(apiPort)), timeout); err == nil {
		return Maintenance, nil
	}
	return Unreachable, fmt.Errorf("node %s is not reachable on :%d or :%d", ip, apiPort, k8sPort)
}
