package podlookup

import "testing"

func TestInferDeploymentFromPodName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		podName    string
		deployment string
	}{
		// Standard Deployment pods: <deploy>-<rs-hash>-<pod-hash>
		{"my-service-7f8d9c-abc12", "my-service"},
		{"api-gateway-5b6c7d8e9f-xz1y2", "api-gateway"},
		{"frontend-abc12def3-ghij4", "frontend"},

		// Multi-hyphen deployment name
		{"my-cool-app-7f8d9c-abc12", "my-cool-app"},

		// StatefulSet pods: <sts>-<ordinal>
		{"redis-master-0", "redis-master"},
		{"kafka-2", "kafka"},

		// DaemonSet / bare pod (no pattern match → returns as-is)
		{"fluent-bit-lxkv8", "fluent-bit-lxkv8"},

		// Single word pod name
		{"standalone", "standalone"},
	}

	for _, tc := range tests {
		t.Run(tc.podName, func(t *testing.T) {
			t.Parallel()
			got := InferDeploymentFromPodName(tc.podName)
			if got != tc.deployment {
				t.Errorf("InferDeploymentFromPodName(%q) = %q, want %q", tc.podName, got, tc.deployment)
			}
		})
	}
}

func TestParseCgroupForPodUID(t *testing.T) {
	t.Parallel()

	// This test only verifies the regex logic, not actual /proc access.
	// Direct parsing is tested via the regex patterns.

	tests := []struct {
		name        string
		input       string
		wantUID     string
		wantCID     string
	}{
		{
			name:    "containerd format",
			input:   "12:memory:/kubepods/burstable/pod12345678-1234-1234-1234-123456789012/a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
			wantUID: "12345678-1234-1234-1234-123456789012",
			wantCID: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		},
		{
			name:    "CRI-O format",
			input:   "1:name=systemd:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod12345678_1234_1234_1234_123456789012.slice/crio-a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2.scope",
			wantUID: "12345678-1234-1234-1234-123456789012",
			wantCID: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		},
		{
			name:    "no kubernetes",
			input:   "12:memory:/user.slice/user-1000.slice",
			wantUID: "",
			wantCID: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Test the regex patterns directly.
			var gotUID, gotCID string

			if m := podUIDPattern.FindStringSubmatch(tc.input); len(m) > 1 {
				// Normalize underscores to hyphens like the real function does.
				uid := m[1]
				for i := range uid {
					if uid[i] == '_' {
						uid = uid[:i] + "-" + uid[i+1:]
					}
				}
				gotUID = uid
			}

			if m := crioContainerPattern.FindStringSubmatch(tc.input); len(m) > 1 {
				gotCID = m[1]
			} else if m := containerIDPattern.FindStringSubmatch(tc.input); len(m) > 1 {
				gotCID = m[1]
			}

			if gotUID != tc.wantUID {
				t.Errorf("podUID = %q, want %q", gotUID, tc.wantUID)
			}
			if gotCID != tc.wantCID {
				t.Errorf("containerID = %q, want %q", gotCID, tc.wantCID)
			}
		})
	}
}

func TestShortContainerID(t *testing.T) {
	t.Parallel()

	if got := shortContainerID("a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"); got != "a1b2c3d4e5f6" {
		t.Errorf("got %q, want 12-char prefix", got)
	}
	if got := shortContainerID("short"); got != "short" {
		t.Errorf("got %q, want %q", got, "short")
	}
}
