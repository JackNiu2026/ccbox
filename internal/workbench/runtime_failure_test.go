package workbench

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestRunCagePreservesOriginalVMError(t *testing.T) {
	if os.Getenv("CAGE_TEST_START_FAILURE_HELPER") == "1" {
		fmt.Fprintln(os.Stderr, "Error: QEMU 启动失败：WHPX: Failed to enable nested virtualization, hr=80370302 token=private-test-token")
		fmt.Fprintln(os.Stderr, "Error: Failed to start base VM. Full log: local diagnostic logs")
		os.Exit(7)
	}
	t.Setenv("CAGE_TEST_START_FAILURE_HELPER", "1")
	s := testServer(t)
	if err := writePrivate(s.profile+string(os.PathSeparator)+"environment.json", map[string]string{"directory": t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	s.exe = exe
	if _, err := s.submitJob("setup", func() error { return s.runCage("-test.run=^TestRunCagePreservesOriginalVMError$") }); err != nil {
		t.Fatal(err)
	}
	job := awaitJobIdle(t, &s.jobs)
	if !strings.Contains(job.Error, "80370302") || strings.Contains(job.Error, "private-test-token") || !strings.Contains(job.Log, "Failed to start base VM") {
		t.Fatalf("lost or exposed the original error: %+v", job)
	}
}

func TestRunCageExplainsCommonFailure(t *testing.T) {
	if output := os.Getenv("CAGE_TEST_READABLE_FAILURE"); output != "" {
		fmt.Fprintln(os.Stderr, output)
		os.Exit(7)
	}
	for _, tc := range []struct{ output, want string }{
		{"curl: (28) Operation timed out", "超时"},
		{"E: No space left on device", "空间不足"},
		{"curl: (60) certificate verify failed", "证书验证"},
		{"Error: Base VM not found. Run 'agent-vm setup' first.", "基础模板缺失"},
		{"Error: Base VM setup did not complete. Run 'agent-vm setup' again.", "基础模板尚未安装完成"},
		{"CAGE_WINDOWS_MAINTENANCE_HOST_MANAGER synthetic stage token=private-error-secret", "系统管理助手安装未完成"},
		{"CAGE_WINDOWS_MAINTENANCE_GUEST_CONNECTION synthetic stage token=private-error-secret", "客机连接未完成"},
		{"unrecognized failure token=private-error-secret", "退出码 7"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			t.Setenv("CAGE_TEST_READABLE_FAILURE", tc.output)
			s := testServer(t)
			if err := writePrivate(s.profile+string(os.PathSeparator)+"environment.json", map[string]string{"directory": t.TempDir()}); err != nil {
				t.Fatal(err)
			}
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			s.exe = exe
			if _, err := s.submitJob("setup", func() error { return s.runCage("-test.run=^TestRunCageExplainsCommonFailure$") }); err != nil {
				t.Fatal(err)
			}
			job := awaitJobIdle(t, &s.jobs)
			if !strings.Contains(job.Error, tc.want) || strings.Contains(job.Error, "private-error-secret") || !strings.Contains(job.Message, "准备环境") {
				t.Fatalf("missing readable operation failure: %+v", job)
			}
		})
	}
}

func TestRunCageDistinguishesBaseBootFromImageDownload(t *testing.T) {
	if output := os.Getenv("CAGE_TEST_BASE_BOOT_FAILURE"); output != "" {
		fmt.Fprintln(os.Stderr, "CAGE_IMAGE_READY")
		fmt.Fprintln(os.Stderr, output)
		os.Exit(1)
	}
	for _, output := range []string{
		"Error: failed to start: host agent process has exited: exit status 1",
		"Error: exiting, status={Running:false Degraded:false Exiting:true Errors:[] SSHLocalPort:0 CloudInitProgress:<nil> PortForward:<nil> Vsock:<nil>} (hint: see [home path])",
	} {
		t.Run(output, func(t *testing.T) {
			assertBaseBootFailure(t, output)
		})
	}
}

func assertBaseBootFailure(t *testing.T, output string) {
	t.Helper()
	t.Setenv("CAGE_TEST_BASE_BOOT_FAILURE", output)
	s := testServer(t)
	if err := writePrivate(s.profile+string(os.PathSeparator)+"environment.json", map[string]string{"directory": t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	s.exe = exe
	if _, err := s.submitJob("setup", func() error {
		s.jobs.execution().setSetupProgress(&SetupProgress{Phase: "image", Label: "正在准备 Debian 13 镜像"})
		return s.runCage("-test.run=^TestRunCageDistinguishesBaseBootFromImageDownload$")
	}); err != nil {
		t.Fatal(err)
	}
	job := awaitJobIdle(t, &s.jobs)
	if job.SetupProgress == nil || job.SetupProgress.Operation != "boot" ||
		!strings.Contains(job.Error, "虚拟机后台在环境就绪前退出") || strings.Contains(job.Error, "未返回可识别的原因") {
		t.Fatalf("base VM startup failure was not distinguished: %+v", job)
	}
}
