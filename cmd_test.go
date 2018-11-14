package taskeeper

import (
	"os"
	"runtime"
	"testing"
)

func TestCmd(t *testing.T) {
	var cmdstr string
	switch runtime.GOOS {
	case "windows":
		cmdstr = "keeper/test/test.exe"
	case "darwin":
		cmdstr = "keeper/test/test_darwin"
	case "linux":
		cmdstr = "keeper/test/test_linux"
	}
	cmd := NewCommand(cmdstr, []string{" hello ", " world "}, "test/cmd.test.log")

	output := cmd.Output()
	t.Logf("output : %s\n", output)
	pid := cmd.Start()
	t.Logf("start pid : %d real pid :%d", pid, cmd.Pid())
	if pid > 0 {
		p := cmd.Process()
		t.Logf("process :%#v\n", p)
		err := cmd.Kill()
		t.Logf("kill res : %#v\n", err)
		process, err := cmd.Wait()
		t.Logf("process :%T err: %#v \n", process, err)
		err = cmd.Release()
		t.Logf("release res : %#v\n", err)
		err = cmd.Singal(os.Interrupt)
		t.Logf("signal res: %#v\n", err)
	}
	cmd.SetCron("* * * * *")
	isCron := cmd.IsCron()
	t.Logf("iscron : %#v\n", isCron)
	cmd.SetId("testId")
	id := cmd.ID()
	t.Logf("cmd id: %s \n", id)

}
