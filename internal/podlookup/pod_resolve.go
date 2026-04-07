package podlookup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// PodInfo holds resolved K8s pod metadata.
type PodInfo struct {
	ContainerID  string
	PodName      string
	PodNamespace string
	Deployment   string
}

// ResolvePodInfo attempts to resolve K8s pod info from a PID.
// It tries multiple strategies: cgroup parsing, /proc/environ, crictl.
func ResolvePodInfo(pid int) *PodInfo {
	podUID, containerID := parseCgroupForPodUID(pid)
	if podUID == "" && containerID == "" {
		return nil // Not a K8s container.
	}

	info := &PodInfo{ContainerID: shortContainerID(containerID)}

	// Strategy 1: Read HOSTNAME from /proc/{pid}/environ (fast, no exec).
	if podName := readPodNameFromEnviron(pid); podName != "" {
		info.PodName = podName
		info.Deployment = InferDeploymentFromPodName(podName)
	}

	// Strategy 2: Try crictl for richer info.
	if containerID != "" {
		if crictlInfo := resolvePodViaCrictl(containerID); crictlInfo != nil {
			if crictlInfo.PodName != "" {
				info.PodName = crictlInfo.PodName
			}
			if crictlInfo.PodNamespace != "" {
				info.PodNamespace = crictlInfo.PodNamespace
			}
			if crictlInfo.Deployment != "" {
				info.Deployment = crictlInfo.Deployment
			} else if info.Deployment == "" && info.PodName != "" {
				info.Deployment = InferDeploymentFromPodName(info.PodName)
			}
		}
	}

	// Fallback: infer deployment from pod name if still empty.
	if info.Deployment == "" && info.PodName != "" {
		info.Deployment = InferDeploymentFromPodName(info.PodName)
	}

	return info
}

// podUIDPattern matches pod UID in cgroup paths.
// containerd: /kubepods/burstable/pod<uid>/<container-id>
// CRI-O: /kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod<uid>.slice/crio-<container-id>.scope
var podUIDPattern = regexp.MustCompile(`pod([0-9a-f]{8}[-_][0-9a-f]{4}[-_][0-9a-f]{4}[-_][0-9a-f]{4}[-_][0-9a-f]{12})`)

// containerIDPattern matches container IDs (64-char hex) in cgroup paths.
var containerIDPattern = regexp.MustCompile(`([0-9a-f]{64})`)

// crioContainerPattern matches CRI-O container IDs: crio-<id>.scope
var crioContainerPattern = regexp.MustCompile(`crio-([0-9a-f]{64})\.scope`)

func parseCgroupForPodUID(pid int) (podUID, containerID string) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return "", ""
	}

	content := string(data)

	// Extract pod UID.
	if m := podUIDPattern.FindStringSubmatch(content); len(m) > 1 {
		podUID = strings.ReplaceAll(m[1], "_", "-")
	}

	// Extract container ID (CRI-O format first, then generic).
	if m := crioContainerPattern.FindStringSubmatch(content); len(m) > 1 {
		containerID = m[1]
	} else if m := containerIDPattern.FindStringSubmatch(content); len(m) > 1 {
		containerID = m[1]
	}

	return podUID, containerID
}

func readPodNameFromEnviron(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return ""
	}

	// /proc/pid/environ uses null bytes as separators.
	for _, entry := range bytes.Split(data, []byte{0}) {
		if bytes.HasPrefix(entry, []byte("HOSTNAME=")) {
			return string(entry[9:])
		}
	}
	return ""
}

type crictlInspectResult struct {
	Status struct {
		Labels map[string]string `json:"labels"`
	} `json:"status"`
	Info struct {
		Config struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Labels map[string]string `json:"labels"`
		} `json:"config"`
		SandboxID string `json:"sandboxId"`
	} `json:"info"`
}

type crictlPodResult struct {
	Status struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Labels map[string]string `json:"labels"`
	} `json:"status"`
}

func resolvePodViaCrictl(containerID string) *PodInfo {
	// Try crictl inspect to get container info.
	out, err := exec.Command("crictl", "inspect", "-o", "json", containerID).Output()
	if err != nil {
		return nil
	}

	var inspect crictlInspectResult
	if err := json.Unmarshal(out, &inspect); err != nil {
		return nil
	}

	info := &PodInfo{ContainerID: shortContainerID(containerID)}

	// Get pod name from container labels.
	if name := inspect.Status.Labels["io.kubernetes.pod.name"]; name != "" {
		info.PodName = name
	}
	if ns := inspect.Status.Labels["io.kubernetes.pod.namespace"]; ns != "" {
		info.PodNamespace = ns
	}

	// Try to get deployment from pod sandbox.
	sandboxID := inspect.Info.SandboxID
	if sandboxID != "" {
		if podInfo := resolvePodSandbox(sandboxID); podInfo != nil {
			if podInfo.PodName != "" && info.PodName == "" {
				info.PodName = podInfo.PodName
			}
			if podInfo.PodNamespace != "" && info.PodNamespace == "" {
				info.PodNamespace = podInfo.PodNamespace
			}
			info.Deployment = podInfo.Deployment
		}
	}

	return info
}

func resolvePodSandbox(sandboxID string) *PodInfo {
	out, err := exec.Command("crictl", "inspectp", "-o", "json", sandboxID).Output()
	if err != nil {
		return nil
	}

	var pod crictlPodResult
	if err := json.Unmarshal(out, &pod); err != nil {
		return nil
	}

	info := &PodInfo{
		PodName:      pod.Status.Metadata.Name,
		PodNamespace: pod.Status.Metadata.Namespace,
	}

	// Try common label conventions for deployment name.
	labels := pod.Status.Labels
	for _, key := range []string{
		"app",
		"app.kubernetes.io/name",
		"app.kubernetes.io/instance",
	} {
		if v := labels[key]; v != "" {
			info.Deployment = v
			break
		}
	}

	return info
}

// InferDeploymentFromPodName strips the ReplicaSet hash and pod hash suffix
// from a pod name to infer the deployment name.
// E.g. "my-service-7f8d9c-abc12" → "my-service"
func InferDeploymentFromPodName(podName string) string {
	// K8s pod names: <deployment>-<replicaset-hash>-<pod-hash>
	// ReplicaSet hash: 5-10 alphanumeric chars
	// Pod hash: 5 alphanumeric chars
	re := regexp.MustCompile(`^(.+)-[a-z0-9]{5,10}-[a-z0-9]{4,5}$`)
	if m := re.FindStringSubmatch(podName); len(m) > 1 {
		return m[1]
	}

	// StatefulSet: <statefulset>-<ordinal>
	re2 := regexp.MustCompile(`^(.+)-\d+$`)
	if m := re2.FindStringSubmatch(podName); len(m) > 1 {
		return m[1]
	}

	return podName
}

func shortContainerID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
