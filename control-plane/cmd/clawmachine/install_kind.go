package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func kindContextName(clusterName string) string {
	return "kind-" + clusterName
}

func createKindCluster(clusterName string, useCilium bool) error {
	if !cmdExists("kind") {
		return fmt.Errorf("kind is required to create a local cluster")
	}

	exists, err := kindClusterExists(clusterName)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("kind cluster %q already exists; delete it first with: kind delete cluster --name %s", clusterName, clusterName)
	}

	configPath, cleanup, err := writeKindConfigForInstall(useCilium)
	if err != nil {
		return err
	}
	defer cleanup()

	styledPrintf(accentStyle, "Creating kind cluster %q...", clusterName)
	args := []string{
		"create", "cluster",
		"--name", clusterName,
		"--config", configPath,
	}
	command := exec.Command("kind", args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

	if err := command.Run(); err != nil {
		return fmt.Errorf("creating kind cluster %q: %w", clusterName, err)
	}

	styledPrintf(successStyle, "kind cluster %q ready.", clusterName)
	return nil
}

func kindClusterExists(clusterName string) (bool, error) {
	out, err := exec.Command("kind", "get", "clusters").Output()
	if err != nil {
		return false, fmt.Errorf("listing kind clusters: %w", err)
	}

	for line := range strings.SplitSeq(string(out), "\n") {
		if strings.TrimSpace(line) == clusterName {
			return true, nil
		}
	}
	return false, nil
}

func writeKindConfigForInstall(useCilium bool) (string, func(), error) {
	data := kindConfigDefaultCNI
	if useCilium {
		data = kindConfigCilium
	}

	tmp, err := os.CreateTemp("", "clawmachine-kind-*.yaml")
	if err != nil {
		return "", nil, fmt.Errorf("creating temp kind config: %w", err)
	}

	if _, err := tmp.Write(data); err != nil {
		if closeErr := tmp.Close(); closeErr != nil {
			return "", nil, fmt.Errorf("writing temp kind config: %w (close: %v)", err, closeErr)
		}
		return "", nil, fmt.Errorf("writing temp kind config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", nil, fmt.Errorf("closing temp kind config: %w", err)
	}

	cleanup := func() {
		_ = os.Remove(tmp.Name())
	}
	return tmp.Name(), cleanup, nil
}
