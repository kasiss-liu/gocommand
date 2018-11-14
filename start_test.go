package taskeeper

import (
	"net"
	"testing"
	"time"
)

func TestStart(t *testing.T) {
	ok := SetWorkDir("/usr/src/app/src/github.com/kasiss-liu/taskeeper/keeper")
	t.Log("set workdir ", ok)
	go func() {
		time.Sleep(3 * time.Second)
		msg, err := sendSignal("reload")
		t.Log(msg, err)
	}()
	go func() {
		time.Sleep(6 * time.Second)
		msg, err := sendSignal("exit")
		t.Log(msg, err)
	}()
	go func() {
		msg, err := sendSignal("test")
		t.Log(msg, err)
		serr := setOutput(logPath)
		if serr != nil {
			t.Log("serr:" + serr.Error())
		}
		time.Sleep(1 * time.Second)
		rs := getRunningStatus()
		t.Log("runstatus:", rs)
		cmdList := getCmdList()
		t.Log("cmd list :", cmdList)
		for id, _ := range cmds {
			msgProcess([]byte(`stat cmd ` + id))
			break
		}
	}()

	go func() {
		time.Sleep(1 * time.Second)
		conn, err := net.Dial("tcp", configPort)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer conn.Close()
		n, err := conn.Write([]byte(`stat f server\n`))
		if err != nil {
			t.Log(err.Error())
		} else {
			t.Log("write :", n)
		}
		var buf = make([]byte, 1000)
		rn, err := conn.Read(buf)
		if err != nil {
			t.Log(err.Error())
		} else {
			t.Log(string(buf[:rn]))
		}

	}()
	go func() {
		time.Sleep(1 * time.Second)
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer conn.Close()
		n, err := conn.Write([]byte(`stat server\n`))
		if err != nil {
			t.Log(err.Error())
		} else {
			t.Log("write :", n)
		}
		var buf = make([]byte, 1000)
		rn, err := conn.Read(buf)
		if err != nil {
			t.Log(err.Error())
		} else {
			t.Log(string(buf[:rn]))
		}

	}()
	Start("config/config.yml", false, true)
}
