package selfupdate

import (
	"os"
	"testing"
)

func TestProcessAliveProbe(t *testing.T) {
	t.Logf("self pid %d alive=%v", os.Getpid(), processAlive(os.Getpid()))
	t.Logf("pid 4 alive=%v", processAlive(4))
	t.Logf("pid 999999 alive=%v", processAlive(999999))
	if !processAlive(os.Getpid()) {
		t.Fatal("processAlive reported the current process as dead")
	}
}
