/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package waitforready implements the wait for ready action
package waitforready

import (
	"fmt"
	"strings"
	"time"

	"sigs.k8s.io/kind/pkg/cluster/nodes"
	"sigs.k8s.io/kind/pkg/cluster/nodeutils"
	"sigs.k8s.io/kind/pkg/exec"
	"sigs.k8s.io/kind/pkg/internal/cluster/create/actions"
)

// Action implements an action for waiting for the cluster to be ready
type Action struct {
	waitTime time.Duration
}

// NewAction returns a new action for waiting for the cluster to be ready
func NewAction(waitTime time.Duration) actions.Action {
	return &Action{
		waitTime: waitTime,
	}
}

// Execute runs the action
func (a *Action) Execute(ctx *actions.ActionContext) error {
	// skip entirely if the wait time is 0
	if a.waitTime == time.Duration(0) {
		return nil
	}
	ctx.Status.Start(
		fmt.Sprintf(
			"Waiting ≤ %s for control-plane = Ready ⏳",
			formatDuration(a.waitTime),
		),
	)

	allNodes, err := ctx.Nodes()
	if err != nil {
		return err
	}
	// get a control plane node to use to check cluster status
	controlPlanes, err := nodeutils.ControlPlaneNodes(allNodes)
	if err != nil {
		return err
	}
	node := controlPlanes[0] // kind expects at least one always

	// Wait for the nodes to reach Ready status.
	startTime := time.Now()
	isReady := waitForReady(node, startTime.Add(a.waitTime))
	if !isReady {
		ctx.Status.End(false)
		fmt.Println(" • WARNING: Timed out waiting for Ready ⚠️")
		return nil
	}

	// mark success
	ctx.Status.End(true)
	fmt.Printf(" • Ready after %s 💚\n", formatDuration(time.Since(startTime)))
	return nil
}

// WaitForReady uses kubectl inside the "node" container to check if the
// control plane nodes are "Ready".
func waitForReady(node nodes.Node, until time.Time) bool {
	return tryUntil(until, func() bool {
		cmd := node.Command(
			"kubectl",
			"--kubeconfig=/etc/kubernetes/admin.conf",
			"get",
			"nodes",
			"--selector=node-role.kubernetes.io/master",
			// When the node reaches status ready, the status field will be set
			// to true.
			"-o=jsonpath='{.items..status.conditions[-1:].status}'",
		)
		lines, err := exec.CombinedOutputLines(cmd)
		if err != nil {
			return false
		}

		// 'lines' will return the status of all nodes labeled as master. For
		// example, if we have three control plane nodes, and all are ready,
		// then the status will have the following format: `True True True'.
		status := strings.Fields(lines[0])
		for _, s := range status {
			// Check node status. If node is ready then this wil be 'True',
			// 'False' or 'Unkown' otherwise.
			if !strings.Contains(s, "True") {
				return false
			}
		}
		return true
	})
}

// helper that calls `try()`` in a loop until the deadline `until`
// has passed or `try()`returns true, returns wether try ever returned true
func tryUntil(until time.Time, try func() bool) bool {
	for until.After(time.Now()) {
		if try() {
			return true
		}
	}
	return false
}

func formatDuration(duration time.Duration) string {
	return duration.Round(time.Second).String()
}
